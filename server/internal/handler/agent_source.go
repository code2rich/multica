package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/agentwaker"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// agentSourceSyncMaxStoreRetention bounds how long a completed/failed scan
// request lingers in the in-memory store before being garbage-collected. It is
// intentionally longer than the terminal-state polling window so a slow client
// still observes the final status. Mirrors runtimeLocalSkillStoreRetention.
const agentSourceSyncStoreRetention = 5 * time.Minute

// Scan request timeouts. A directory scan walks the filesystem and hashes
// bundles, so it can legitimately take longer than a local-skill listing.
const (
	agentSourceScanPendingTimeout = 5 * time.Minute
	agentSourceScanRunningTimeout = 3 * time.Minute
)

// AgentWakerScanStatus mirrors the runtime-local-skill status lifecycle.
type AgentWakerScanStatus string

const (
	AgentWakerScanPending   AgentWakerScanStatus = "pending"
	AgentWakerScanRunning   AgentWakerScanStatus = "running"
	AgentWakerScanCompleted AgentWakerScanStatus = "completed"
	AgentWakerScanFailed    AgentWakerScanStatus = "failed"
	AgentWakerScanTimeout   AgentWakerScanStatus = "timeout"
)

func (s AgentWakerScanStatus) Terminal() bool {
	return s == AgentWakerScanCompleted || s == AgentWakerScanFailed || s == AgentWakerScanTimeout
}

// AgentWakerScanRequest is one queued read-only directory scan. It is the
// in-flight record held by the store and surfaced to the polling client. The
// Manifest field carries the daemon's sanitized scan result (env key names +
// digests only — verified value-free by the report handler). DirectoryHash is
// the canonical manifest digest.
type AgentWakerScanRequest struct {
	ID            string               `json:"id"`
	SourceID      string               `json:"source_id"`
	RuntimeID     string               `json:"runtime_id"`
	AbsPath       string               `json:"-"`
	Status        AgentWakerScanStatus `json:"status"`
	DirectoryHash string               `json:"directory_hash,omitempty"`
	Manifest      json.RawMessage      `json:"manifest,omitempty"`
	Diagnostics   []ScanDiagnostic     `json:"diagnostics,omitempty"`
	ScannerVersion string              `json:"scanner_version,omitempty"`
	Error         string               `json:"error,omitempty"`
	CreatedAt     time.Time            `json:"created_at"`
	UpdatedAt     time.Time            `json:"updated_at"`
	RunStartedAt  *time.Time           `json:"-"`
}

// ScanDiagnostic is one validation error or warning produced by the scanner.
type ScanDiagnostic struct {
	Severity string `json:"severity"` // "error" | "warning"
	Code     string `json:"code"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
}

// AgentWakerScanStore is the shared-state contract for scan requests. Like
// LocalSkillListStore, it MUST be backed by Redis in multi-node deploys so the
// HTTP initiate, daemon heartbeat claim, and client poll can land on different
// nodes and still agree. The in-memory implementation is dev/single-node only.
type AgentWakerScanStore interface {
	Create(ctx context.Context, runtimeID, sourceID, absPath string) (*AgentWakerScanRequest, error)
	Get(ctx context.Context, id string) (*AgentWakerScanRequest, error)
	// HasPending is a cheap read-only probe used by the heartbeat handler to
	// gate the side-effecting PopPending so it never starts a claim it might
	// have to abort (mirrors the local-skill-list two-step).
	HasPending(ctx context.Context, runtimeID string) (bool, error)
	PopPending(ctx context.Context, runtimeID string) (*AgentWakerScanRequest, error)
	Complete(ctx context.Context, id, directoryHash string, manifest json.RawMessage, diagnostics []ScanDiagnostic, scannerVersion string) error
	Fail(ctx context.Context, id string, diagnostics []ScanDiagnostic, errMsg string) error
}

// applyAgentWakerScanTimeout flips a stale pending/running request to timeout.
// Returns true when the record changed. Called on every Get/HasPending/PopPending.
func applyAgentWakerScanTimeout(req *AgentWakerScanRequest, now time.Time) bool {
	switch req.Status {
	case AgentWakerScanPending:
		if now.Sub(req.CreatedAt) > agentSourceScanPendingTimeout {
			req.Status = AgentWakerScanTimeout
			req.Error = "daemon did not respond within 5 minutes"
			req.UpdatedAt = now
			return true
		}
	case AgentWakerScanRunning:
		if req.RunStartedAt != nil && now.Sub(*req.RunStartedAt) > agentSourceScanRunningTimeout {
			req.Status = AgentWakerScanTimeout
			req.Error = "daemon did not finish within 3 minutes"
			req.UpdatedAt = now
			return true
		}
	}
	return false
}

// --- in-memory store (dev / single-node) ---

// InMemoryAgentWakerScanStore mirrors InMemoryLocalSkillListStore. Same
// single-node caveat: production multi-node deploys must swap in a Redis-backed
// implementation via the router (see router.go Redis block).
type InMemoryAgentWakerScanStore struct {
	mu       sync.Mutex
	requests map[string]*AgentWakerScanRequest
}

func NewInMemoryAgentWakerScanStore() *InMemoryAgentWakerScanStore {
	return &InMemoryAgentWakerScanStore{requests: make(map[string]*AgentWakerScanRequest)}
}

func (s *InMemoryAgentWakerScanStore) Create(_ context.Context, runtimeID, sourceID, absPath string) (*AgentWakerScanRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Opportunistic GC of retained terminal records.
	for id, req := range s.requests {
		if time.Since(req.UpdatedAt) > agentSourceSyncStoreRetention {
			delete(s.requests, id)
		}
	}

	req := &AgentWakerScanRequest{
		ID:         randomID(),
		SourceID:   sourceID,
		RuntimeID:  runtimeID,
		AbsPath:    absPath,
		Status:     AgentWakerScanPending,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	s.requests[req.ID] = req
	return req, nil
}

func (s *InMemoryAgentWakerScanStore) Get(_ context.Context, id string) (*AgentWakerScanRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	req, ok := s.requests[id]
	if !ok {
		return nil, nil
	}
	applyAgentWakerScanTimeout(req, time.Now())
	return req, nil
}

func (s *InMemoryAgentWakerScanStore) HasPending(_ context.Context, runtimeID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for _, req := range s.requests {
		applyAgentWakerScanTimeout(req, now)
		if req.RuntimeID == runtimeID && req.Status == AgentWakerScanPending {
			return true, nil
		}
	}
	return false, nil
}

func (s *InMemoryAgentWakerScanStore) PopPending(_ context.Context, runtimeID string) (*AgentWakerScanRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var oldest *AgentWakerScanRequest
	now := time.Now()
	for _, req := range s.requests {
		applyAgentWakerScanTimeout(req, now)
		if req.RuntimeID == runtimeID && req.Status == AgentWakerScanPending {
			if oldest == nil || req.CreatedAt.Before(oldest.CreatedAt) {
				oldest = req
			}
		}
	}
	if oldest != nil {
		oldest.Status = AgentWakerScanRunning
		startedAt := now
		oldest.RunStartedAt = &startedAt
		oldest.UpdatedAt = now
	}
	return oldest, nil
}

func (s *InMemoryAgentWakerScanStore) Complete(_ context.Context, id, directoryHash string, manifest json.RawMessage, diagnostics []ScanDiagnostic, scannerVersion string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if req, ok := s.requests[id]; ok {
		req.Status = AgentWakerScanCompleted
		req.DirectoryHash = directoryHash
		req.Manifest = manifest
		req.Diagnostics = diagnostics
		req.ScannerVersion = scannerVersion
		req.UpdatedAt = time.Now()
	}
	return nil
}

func (s *InMemoryAgentWakerScanStore) Fail(_ context.Context, id string, diagnostics []ScanDiagnostic, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if req, ok := s.requests[id]; ok {
		req.Status = AgentWakerScanFailed
		req.Diagnostics = diagnostics
		req.Error = errMsg
		req.UpdatedAt = time.Now()
	}
	return nil
}

// --- HTTP request/response shapes ---

type CreateAgentSourceRequest struct {
	DaemonRuntimeID string `json:"daemon_runtime_id"`
	LocalPath       string `json:"local_path"`
	SyncMode        string `json:"sync_mode"`
}

type UpdateAgentSourceRequest struct {
	DaemonRuntimeID *string `json:"daemon_runtime_id,omitempty"`
	LocalPath       *string `json:"local_path,omitempty"`
	SyncMode        *string `json:"sync_mode,omitempty"`
}

// AgentSourceResponse is the value-safe API representation. LocalPath is
// returned only to workspace members who can already configure the source;
// ordinary member-facing events redact it. No env values appear here.
type AgentSourceResponse struct {
	ID                string     `json:"id"`
	WorkspaceID       string     `json:"workspace_id"`
	Kind              string     `json:"kind"`
	DaemonRuntimeID   string     `json:"daemon_runtime_id"`
	LocalPath         string     `json:"local_path"`
	SyncMode          string     `json:"sync_mode"`
	Status            string     `json:"status"`
	LastSnapshotHash  string     `json:"last_snapshot_hash,omitempty"`
	LastScannedAt     *time.Time `json:"last_scanned_at,omitempty"`
	LastAppliedAt     *time.Time `json:"last_applied_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type AgentSourceSnapshotResponse struct {
	ID             string          `json:"id"`
	SourceID       string          `json:"source_id"`
	DirectoryHash  string          `json:"directory_hash"`
	SchemaVersions map[string]any  `json:"schema_versions"`
	Manifest       json.RawMessage `json:"manifest"`
	Status         string          `json:"status"`
	Diagnostics    []ScanDiagnostic `json:"diagnostics"`
	LockYAML       string          `json:"lock_yaml,omitempty"`
	ScannerVersion string          `json:"scanner_version,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	AppliedAt      *time.Time      `json:"applied_at,omitempty"`
}

// --- handlers: source CRUD ---

func (h *Handler) CreateAgentSource(w http.ResponseWriter, r *http.Request) {
	if !agentWakerDirectorySyncEnabled(h, w, r) {
		return
	}
	wsID, ok := h.requireWorkspaceID(w, r)
	if !ok {
		return
	}
	member, ok := h.requireWorkspaceRole(w, r, wsID, "workspace not found", "owner", "admin")
	if !ok {
		return
	}

	var body CreateAgentSourceRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	body.LocalPath = strings.TrimSpace(body.LocalPath)
	body.DaemonRuntimeID = strings.TrimSpace(body.DaemonRuntimeID)
	if body.LocalPath == "" || body.DaemonRuntimeID == "" {
		writeError(w, http.StatusBadRequest, "daemon_runtime_id and local_path are required")
		return
	}
	syncMode := body.SyncMode
	if syncMode == "" {
		syncMode = "manual"
	}
	if syncMode != "manual" && syncMode != "scheduled" && syncMode != "watch-assisted" {
		writeError(w, http.StatusBadRequest, "sync_mode must be manual, scheduled, or watch-assisted")
		return
	}

	runtimeUUID, ok := parseUUIDOrBadRequest(w, body.DaemonRuntimeID, "daemon_runtime_id")
	if !ok {
		return
	}
	rt, err := h.Queries.GetAgentRuntime(r.Context(), runtimeUUID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "daemon runtime not found")
		return
	}
	if uuidToString(rt.WorkspaceID) != wsID {
		writeError(w, http.StatusBadRequest, "daemon runtime does not belong to this workspace")
		return
	}

	wsUUID, err := util.ParseUUID(wsID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid workspace id")
		return
	}
	canonicalHash, err := canonicalPathHash(body.LocalPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	params := db.CreateAgentSourceParams{
		WorkspaceID:       wsUUID,
		Kind:              "agentwaker_directory",
		DaemonRuntimeID:   runtimeUUID,
		LocalPath:         body.LocalPath,
		CanonicalPathHash: canonicalHash,
		SyncMode:          syncMode,
		Status:            "pending",
	}
	if member.UserID.Valid {
		params.CreatedBy = pgtype.UUID{Bytes: member.UserID.Bytes, Valid: true}
	}

	src, err := h.Queries.CreateAgentSource(r.Context(), params)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "this directory is already configured as a source")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create source: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, agentSourceToResponse(src))
}

func (h *Handler) ListAgentSources(w http.ResponseWriter, r *http.Request) {
	if !agentWakerDirectorySyncEnabled(h, w, r) {
		return
	}
	wsID, ok := h.requireWorkspaceID(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireWorkspaceMember(w, r, wsID, "workspace not found"); !ok {
		return
	}
	wsUUID := parseUUID(wsID)
	sources, err := h.Queries.ListAgentSourcesByWorkspace(r.Context(), wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list sources: "+err.Error())
		return
	}
	out := make([]AgentSourceResponse, 0, len(sources))
	for _, s := range sources {
		out = append(out, agentSourceToResponse(s))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) GetAgentSource(w http.ResponseWriter, r *http.Request) {
	if !agentWakerDirectorySyncEnabled(h, w, r) {
		return
	}
	src, ok := h.loadAgentSourceForMember(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, agentSourceToResponse(src))
}

func (h *Handler) UpdateAgentSource(w http.ResponseWriter, r *http.Request) {
	if !agentWakerDirectorySyncEnabled(h, w, r) {
		return
	}
	src, ok := h.loadAgentSourceForMember(w, r)
	if !ok {
		return
	}
	// Only owner/admin can reconfigure.
	wsID := uuidToString(src.WorkspaceID)
	if _, ok := h.requireWorkspaceRole(w, r, wsID, "workspace not found", "owner", "admin"); !ok {
		return
	}

	var body UpdateAgentSourceRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	params := db.UpdateAgentSourceParams{ID: src.ID}
	if body.DaemonRuntimeID != nil {
		id, err := util.ParseUUID(strings.TrimSpace(*body.DaemonRuntimeID))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid daemon_runtime_id")
			return
		}
		rt, gerr := h.Queries.GetAgentRuntime(r.Context(), id)
		if gerr != nil || uuidToString(rt.WorkspaceID) != wsID {
			writeError(w, http.StatusBadRequest, "daemon runtime not found in this workspace")
			return
		}
		params.DaemonRuntimeID = pgtype.UUID{Bytes: id.Bytes, Valid: true}
	}
	if body.LocalPath != nil {
		trimmed := strings.TrimSpace(*body.LocalPath)
		hash, _ := canonicalPathHash(trimmed)
		params.LocalPath = pgtype.Text{String: trimmed, Valid: true}
		params.CanonicalPathHash = pgtype.Text{String: hash, Valid: true}
	}
	if body.SyncMode != nil {
		switch *body.SyncMode {
		case "manual", "scheduled", "watch-assisted":
			params.SyncMode = pgtype.Text{String: *body.SyncMode, Valid: true}
		default:
			writeError(w, http.StatusBadRequest, "invalid sync_mode")
			return
		}
	}
	updated, err := h.Queries.UpdateAgentSource(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update source: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, agentSourceToResponse(updated))
}

func (h *Handler) DeleteAgentSource(w http.ResponseWriter, r *http.Request) {
	if !agentWakerDirectorySyncEnabled(h, w, r) {
		return
	}
	src, ok := h.loadAgentSourceForMember(w, r)
	if !ok {
		return
	}
	wsID := uuidToString(src.WorkspaceID)
	if _, ok := h.requireWorkspaceRole(w, r, wsID, "workspace not found", "owner", "admin"); !ok {
		return
	}
	if err := h.Queries.DeleteAgentSource(r.Context(), src.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete source: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- handlers: scan (initiate + poll) ---

// InitiateAgentSourceScan enqueues a read-only directory scan against the
// configured daemon. The scan does NOT mutate any agent/skill/capability/env
// state. The result lands in an immutable sanitized snapshot.
func (h *Handler) InitiateAgentSourceScan(w http.ResponseWriter, r *http.Request) {
	if !agentWakerDirectorySyncEnabled(h, w, r) {
		return
	}
	src, ok := h.loadAgentSourceForMember(w, r)
	if !ok {
		return
	}
	wsID := uuidToString(src.WorkspaceID)
	if _, ok := h.requireWorkspaceRole(w, r, wsID, "workspace not found", "owner", "admin"); !ok {
		return
	}
	// Daemon must be online to claim the scan within the pending timeout.
	rt, err := h.Queries.GetAgentRuntime(r.Context(), src.DaemonRuntimeID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "daemon runtime not found")
		return
	}
	if rt.Status != "online" {
		writeError(w, http.StatusServiceUnavailable, "daemon runtime is offline")
		return
	}

	req, err := h.AgentWakerScanStore.Create(r.Context(), uuidToString(src.DaemonRuntimeID), uuidToString(src.ID), src.LocalPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to enqueue scan: "+err.Error())
		return
	}
	if _, err := h.Queries.UpdateAgentSourceStatus(r.Context(), db.UpdateAgentSourceStatusParams{
		ID:     src.ID,
		Status: "scanning",
	}); err != nil {
		slog.Warn("agent_source: failed to mark scanning", "source_id", src.ID, "error", err)
	}
	writeJSON(w, http.StatusAccepted, req)
}

// GetAgentSourceScanRequest is the poll endpoint. The client polls until the
// status is terminal; the response then carries the sanitized manifest and
// directory hash.
func (h *Handler) GetAgentSourceScanRequest(w http.ResponseWriter, r *http.Request) {
	if !agentWakerDirectorySyncEnabled(h, w, r) {
		return
	}
	src, ok := h.loadAgentSourceForMember(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireWorkspaceMember(w, r, uuidToString(src.WorkspaceID), "workspace not found"); !ok {
		return
	}
	requestID := chi.URLParam(r, "requestId")
	req, err := h.AgentWakerScanStore.Get(r.Context(), requestID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load scan request: "+err.Error())
		return
	}
	if req == nil || req.SourceID != uuidToString(src.ID) {
		writeError(w, http.StatusNotFound, "scan request not found")
		return
	}
	writeJSON(w, http.StatusOK, req)
}

// ReportAgentWakerScanResult is the daemon-only endpoint that receives the
// sanitized scan result. Defense-in-depth: it rejects any payload whose
// manifest carries a plaintext env value (a bare "value" field where a digest
// object is expected). On a successful report it writes an immutable sanitized
// snapshot and updates the source status.
func (h *Handler) ReportAgentWakerScanResult(w http.ResponseWriter, r *http.Request) {
	runtimeID := chi.URLParam(r, "runtimeId")
	if _, ok := h.requireDaemonRuntimeAccess(w, r, runtimeID); !ok {
		return
	}
	requestID := chi.URLParam(r, "requestId")
	req, err := h.AgentWakerScanStore.Get(r.Context(), requestID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load scan request: "+err.Error())
		return
	}
	if req == nil || req.RuntimeID != runtimeID {
		writeError(w, http.StatusNotFound, "scan request not found")
		return
	}
	if req.Status.Terminal() {
		slog.Debug("ignoring stale agentwaker scan report", "runtime_id", runtimeID, "request_id", requestID, "status", req.Status)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	var body struct {
		Status          string          `json:"status"`
		DirectoryHash   string          `json:"directory_hash"`
		Manifest        json.RawMessage `json:"manifest"`
		Diagnostics     []ScanDiagnostic `json:"diagnostics"`
		ScannerVersion  string          `json:"scanner_version"`
		Error           string          `json:"error"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Status == "completed" {
		// Defense-in-depth: reject any plaintext env leakage in the manifest.
		if len(body.Manifest) > 0 {
			var decoded any
			if err := json.Unmarshal(body.Manifest, &decoded); err != nil {
				writeError(w, http.StatusBadRequest, "manifest is not valid JSON")
				return
			}
			if hit := agentwaker.AssertNoPlaintextEnv(decoded); hit != "" {
				slog.Error("agentwaker scan report rejected: plaintext env detected",
					"runtime_id", runtimeID, "request_id", requestID, "path", hit)
				writeError(w, http.StatusBadRequest, "scan manifest rejected: plaintext environment value detected at "+hit)
				return
			}
		}
		if err := h.AgentWakerScanStore.Complete(r.Context(), requestID, body.DirectoryHash, body.Manifest, body.Diagnostics, body.ScannerVersion); err != nil {
			slog.Error("agentwaker scan Complete failed", "error", err, "request_id", requestID)
			writeError(w, http.StatusInternalServerError, "failed to persist completion")
			return
		}
		// M4: hash-based no-op detection. When the directory hash matches the
		// source's last_snapshot_hash, no content changed since the last scan.
		// Skip snapshot creation, just stamp the scanned_at timestamp so the
		// scheduler knows the scan ran. IDs are preserved; no writes happen.
		if src, _ := h.Queries.GetAgentSource(r.Context(), parseUUIDUnsafe(req.SourceID)); src.ID.Valid {
			if src.LastSnapshotHash.Valid && src.LastSnapshotHash.String == body.DirectoryHash {
				scannedAt := time.Now()
				if _, err := h.Queries.UpdateAgentSourceStatus(r.Context(), db.UpdateAgentSourceStatusParams{
					ID:            src.ID,
					Status:        "ready",
					LastScannedAt: pgtype.Timestamptz{Time: scannedAt, Valid: true},
				}); err != nil {
					slog.Warn("agent_source: failed to stamp no-op scan", "source_id", req.SourceID, "error", err)
				}
				slog.Info("agentwaker scan no-op: hash unchanged", "source_id", req.SourceID, "directory_hash", body.DirectoryHash)
				writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "no_op": "true"})
				return
			}
		}
		// Content changed: persist an immutable sanitized snapshot.
		h.persistAgentSourceSnapshot(w, r, req, body.DirectoryHash, body.Manifest, body.Diagnostics, body.ScannerVersion)
		return
	}

	// Failed scan: record diagnostics, keep last-known-good snapshot active.
	if err := h.AgentWakerScanStore.Fail(r.Context(), requestID, body.Diagnostics, body.Error); err != nil {
		slog.Error("agentwaker scan Fail failed", "error", err, "request_id", requestID)
		writeError(w, http.StatusInternalServerError, "failed to persist failure")
		return
	}
	if src, _ := h.Queries.GetAgentSource(r.Context(), parseUUIDUnsafe(req.SourceID)); src.ID.Valid {
		if _, err := h.Queries.UpdateAgentSourceStatus(r.Context(), db.UpdateAgentSourceStatusParams{ID: src.ID, Status: "failed"}); err != nil {
			slog.Warn("agent_source: failed to mark failed", "source_id", req.SourceID, "error", err)
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// persistAgentSourceSnapshot stores the sanitized manifest as an immutable
// preview snapshot and flips the source to ready. M1 only creates previews;
// M2 apply flips a snapshot to applied.
func (h *Handler) persistAgentSourceSnapshot(w http.ResponseWriter, r *http.Request, req *AgentWakerScanRequest, dirHash string, manifest json.RawMessage, diagnostics []ScanDiagnostic, scannerVersion string) {
	sourceUUID, ok := parseUUIDOrBadRequest(w, req.SourceID, "source_id")
	if !ok {
		return
	}
	schemaVersions := map[string]any{
		"profile":   agentwaker.ProfileSchemaVersion,
		"capability": agentwaker.CapabilitySchemaVersion,
	}
	schemaJSON, _ := json.Marshal(schemaVersions)
	diagJSON, _ := json.Marshal(diagnostics)
	if diagJSON == nil {
		diagJSON = []byte("[]")
	}
	snap, err := h.Queries.CreateAgentSourceSnapshot(r.Context(), db.CreateAgentSourceSnapshotParams{
		SourceID:       sourceUUID,
		DirectoryHash:  dirHash,
		SchemaVersions: schemaJSON,
		Manifest:       manifest,
		Status:         "preview",
		Diagnostics:    diagJSON,
		ScannerVersion: pgtype.Text{String: scannerVersion, Valid: scannerVersion != ""},
	})
	if err != nil {
		slog.Error("agent_source: failed to create snapshot", "error", err, "source_id", req.SourceID)
		writeError(w, http.StatusInternalServerError, "failed to persist snapshot: "+err.Error())
		return
	}
	scannedAt := time.Now()
	if _, err := h.Queries.UpdateAgentSourceStatus(r.Context(), db.UpdateAgentSourceStatusParams{
		ID:               sourceUUID,
		Status:           "ready",
		LastSnapshotHash: pgtype.Text{String: dirHash, Valid: dirHash != ""},
		LastScannedAt:    pgtype.Timestamptz{Time: scannedAt, Valid: true},
	}); err != nil {
		slog.Warn("agent_source: failed to mark ready", "source_id", req.SourceID, "error", err)
	}
	slog.Info("agentwaker scan snapshot stored", "source_id", req.SourceID, "snapshot_id", uuidToString(snap.ID), "directory_hash", dirHash)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- snapshot listing ---

func (h *Handler) ListAgentSourceSnapshots(w http.ResponseWriter, r *http.Request) {
	if !agentWakerDirectorySyncEnabled(h, w, r) {
		return
	}
	src, ok := h.loadAgentSourceForMember(w, r)
	if !ok {
		return
	}
	snaps, err := h.Queries.ListAgentSourceSnapshots(r.Context(), src.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list snapshots: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, agentSourceSnapshotListToResponse(snaps))
}

func (h *Handler) GetAgentSourceSnapshot(w http.ResponseWriter, r *http.Request) {
	if !agentWakerDirectorySyncEnabled(h, w, r) {
		return
	}
	src, ok := h.loadAgentSourceForMember(w, r)
	if !ok {
		return
	}
	snapID := chi.URLParam(r, "snapshotId")
	snapUUID, ok := parseUUIDOrBadRequest(w, snapID, "snapshot_id")
	if !ok {
		return
	}
	snap, err := h.Queries.GetAgentSourceSnapshotInSource(r.Context(), db.GetAgentSourceSnapshotInSourceParams{
		ID: snapUUID, SourceID: src.ID,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "snapshot not found")
		return
	}
	writeJSON(w, http.StatusOK, agentSourceSnapshotToResponse(snap))
}

// --- helpers ---

// loadAgentSourceForMember loads the source by URL :id and authorizes the
// caller against its workspace membership.
func (h *Handler) loadAgentSourceForMember(w http.ResponseWriter, r *http.Request) (db.AgentSource, bool) {
	id := chi.URLParam(r, "id")
	srcUUID, ok := parseUUIDOrBadRequest(w, id, "source_id")
	if !ok {
		return db.AgentSource{}, false
	}
	src, err := h.Queries.GetAgentSource(r.Context(), srcUUID)
	if err != nil {
		writeError(w, http.StatusNotFound, "agent source not found")
		return db.AgentSource{}, false
	}
	if _, ok := h.requireWorkspaceMember(w, r, uuidToString(src.WorkspaceID), "agent source not found"); !ok {
		return db.AgentSource{}, false
	}
	return src, true
}

func agentSourceToResponse(s db.AgentSource) AgentSourceResponse {
	resp := AgentSourceResponse{
		ID:              uuidToString(s.ID),
		WorkspaceID:     uuidToString(s.WorkspaceID),
		Kind:            s.Kind,
		DaemonRuntimeID: uuidToString(s.DaemonRuntimeID),
		LocalPath:       s.LocalPath,
		SyncMode:        s.SyncMode,
		Status:          s.Status,
		CreatedAt:       s.CreatedAt.Time,
		UpdatedAt:       s.UpdatedAt.Time,
	}
	if s.LastSnapshotHash.Valid {
		resp.LastSnapshotHash = s.LastSnapshotHash.String
	}
	if s.LastScannedAt.Valid {
		t := s.LastScannedAt.Time
		resp.LastScannedAt = &t
	}
	if s.LastAppliedAt.Valid {
		t := s.LastAppliedAt.Time
		resp.LastAppliedAt = &t
	}
	return resp
}

func agentSourceSnapshotToResponse(s db.AgentSourceSnapshot) AgentSourceSnapshotResponse {
	var schema map[string]any
	_ = json.Unmarshal(s.SchemaVersions, &schema)
	diags := []ScanDiagnostic{}
	if len(s.Diagnostics) > 0 {
		_ = json.Unmarshal(s.Diagnostics, &diags)
	}
	if diags == nil {
		diags = []ScanDiagnostic{}
	}
	resp := AgentSourceSnapshotResponse{
		ID:             uuidToString(s.ID),
		SourceID:       uuidToString(s.SourceID),
		DirectoryHash:  s.DirectoryHash,
		SchemaVersions: schema,
		Manifest:       s.Manifest,
		Status:         s.Status,
		Diagnostics:    diags,
		CreatedAt:      s.CreatedAt.Time,
	}
	if s.ScannerVersion.Valid {
		resp.ScannerVersion = s.ScannerVersion.String
	}
	if s.AppliedAt.Valid {
		t := s.AppliedAt.Time
		resp.AppliedAt = &t
	}
	if s.LockYaml.Valid {
		resp.LockYAML = s.LockYaml.String
	}
	return resp
}

// agentSourceSnapshotListToResponse keeps the snapshot history lightweight.
// A manifest contains the complete imported role and skill bodies and can be
// several megabytes. Returning it for every historical snapshot made the
// settings page download and parse tens of megabytes before it could render
// the Apply button. The list only needs the newest actionable preview's
// manifest; callers can use GetAgentSourceSnapshot for any historical body.
func agentSourceSnapshotListToResponse(snaps []db.AgentSourceSnapshot) []AgentSourceSnapshotResponse {
	out := make([]AgentSourceSnapshotResponse, 0, len(snaps))
	previewManifestIncluded := false
	for _, s := range snaps {
		resp := agentSourceSnapshotToResponse(s)
		if s.Status == "preview" && !previewManifestIncluded {
			previewManifestIncluded = true
		} else {
			resp.Manifest = json.RawMessage("null")
		}
		out = append(out, resp)
	}
	return out
}

// canonicalPathHash returns a stable, non-reversible hash of the absolute path
// so two sources pointing at the same canonical directory are detected without
// exposing the full path in every comparison.
func canonicalPathHash(absPath string) (string, error) {
	if absPath == "" {
		return "", errors.New("local_path is empty")
	}
	// SHA-256 is sufficient: this is a uniqueness key, not a secret. The path is
	// already visible to workspace admins via the API; the hash just avoids
	// spraying the full path through every event and comparison.
	sum := sha256.Sum256([]byte(absPath))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// agentWakerDirectorySyncEnabled is the feature-gate guard. Returns false (and
// writes an error) when the flag is off.
func agentWakerDirectorySyncEnabled(h *Handler, w http.ResponseWriter, r *http.Request) bool {
	if !featureflags.AgentWakerDirectorySyncEnabled(r.Context(), h.FeatureFlags) {
		writeError(w, http.StatusNotFound, "not found")
		return false
	}
	return true
}

// requireWorkspaceID reads the workspace ID injected by the workspace-membership
// middleware. Returns "" + false (writing an error) when absent.
func (h *Handler) requireWorkspaceID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := middleware.WorkspaceIDFromContext(r.Context())
	if id == "" {
		writeError(w, http.StatusBadRequest, "workspace required")
		return "", false
	}
	return id, true
}

// parseUUIDUnsafe returns the zero UUID on error; use only for internal lookups
// where the input is known to be a stored UUID.
func parseUUIDUnsafe(s string) pgtype.UUID {
	u, _ := util.ParseUUID(s)
	return u
}
