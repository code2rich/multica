package handler

import (
	"context"
	"log/slog"
	"time"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// agentwakerSyncInterval is how often the server checks for schedulable
// AgentWaker sources. Sources with sync_mode='scheduled' and status in
// (ready, failed) are scanned on this interval. The scan itself is canonical
// (full manifest rescan); the hash-based no-op detection skips snapshot
// creation when nothing changed.
//
// 5 minutes balances timely detection against filesystem-walk cost.
// Watch-assisted detection (filesystem watcher hints) may reduce effective
// latency in the future, but the canonical rescan always runs after any hint.
const agentwakerSyncInterval = 5 * time.Minute

// AgentWakerSyncLoop periodically checks for AgentWaker sources configured for
// scheduled sync and triggers scans. It runs on the server side alongside
// other background jobs. Scan requests are enqueued into the
// AgentWakerScanStore and delivered to daemons via the heartbeat protocol.
//
// A source is "schedulable" when sync_mode='scheduled' AND status is 'ready'
// or 'failed' (not 'pending'/'scanning'/'applying' — those are already in
// progress). The query ListSchedulableAgentSources implements this filter.
//
// Watch-assisted mode (sync_mode='watch-assisted') is treated as manual until
// the watcher integration lands in a later milestone.
func (h *Handler) AgentWakerSyncLoop(ctx context.Context) {
	// Initial delay to let the server finish booting.
	select {
	case <-ctx.Done():
		return
	case <-time.After(30 * time.Second):
	}

	ticker := time.NewTicker(agentwakerSyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.pollScheduledAgentSources(ctx)
		}
	}
}

// pollScheduledAgentSources finds all schedulable sources across all workspaces
// and enqueues a scan request for each one. Scan requests that are already
// pending/running for a source are naturally deduplicated by the store's
// HasPending check.
func (h *Handler) pollScheduledAgentSources(ctx context.Context) {
	sources, err := h.Queries.ListSchedulableAgentSources(ctx)
	if err != nil {
		// Log but don't crash the loop — transient DB errors recover on the
		// next tick.
		slog.Error("agentwaker sync: failed to list schedulable sources", "error", err)
		return
	}
	if len(sources) == 0 {
		return
	}
	for _, src := range sources {
		runtimeID := uuidToString(src.DaemonRuntimeID)
		sourceID := uuidToString(src.ID)
		absPath := src.LocalPath
		// Check if there's already a pending/running scan for this source.
		hasPending, hpErr := h.AgentWakerScanStore.HasPending(ctx, runtimeID)
		if hpErr != nil {
			slog.Warn("agentwaker sync: HasPending check failed", "source_id", sourceID, "error", hpErr)
			continue
		}
		if hasPending {
			slog.Debug("agentwaker sync: scan already pending, skipping", "source_id", sourceID)
			continue
		}
		if _, err := h.AgentWakerScanStore.Create(ctx, runtimeID, sourceID, absPath); err != nil {
			slog.Error("agentwaker sync: failed to enqueue scan", "source_id", sourceID, "error", err)
			continue
		}
		// Mark the source as scanning so the UI shows the status transition.
		if _, err := h.Queries.UpdateAgentSourceStatus(ctx, db.UpdateAgentSourceStatusParams{
			ID:     src.ID,
			Status: "scanning",
		}); err != nil {
			slog.Warn("agentwaker sync: failed to mark scanning", "source_id", sourceID, "error", err)
		}
		slog.Info("agentwaker sync: scheduled scan enqueued", "source_id", sourceID, "runtime_id", runtimeID)
	}
}

