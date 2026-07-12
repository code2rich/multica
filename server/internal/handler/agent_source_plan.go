package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// AgentSourcePlan is the read-only diff between one snapshot's manifest and the
// currently-applied state of the source. It drives the Preview UI and the
// /plan endpoint. Every field is value-free.
type AgentSourcePlan struct {
	Capabilities []AgentSourceChange `json:"capabilities"`
	Roles        []AgentSourceChange `json:"roles"`
	Skills       []AgentSourceChange `json:"skills"`
	Bindings     []AgentSourceChange `json:"bindings"`
	Env          AgentSourceEnvApply `json:"env"`
	MCP          []AgentSourceChange `json:"mcp"`
	// FromHash/ToHash identify the compared states. FromHash is empty when the
	// source has no prior applied snapshot (first import).
	FromHash string `json:"from_hash,omitempty"`
	ToHash   string `json:"to_hash"`
	// BlockingIssues lists problems that would prevent a clean apply (missing
	// required capabilities, unresolved MCP env refs, etc.).
	BlockingIssues []ScanDiagnostic `json:"blocking_issues,omitempty"`
}

// BuildPlan computes the diff between the supplied snapshot and the source's
// current applied state. It performs NO writes — it is the read-only preview
// the UI shows before Apply.
func (h *Handler) BuildPlan(ctx context.Context, sourceID, snapshotID pgtype.UUID) (*AgentSourcePlan, error) {
	snap, err := h.Queries.GetAgentSourceSnapshotInSource(ctx, db.GetAgentSourceSnapshotInSourceParams{
		ID: snapshotID, SourceID: sourceID,
	})
	if err != nil {
		return nil, fmt.Errorf("plan: load snapshot: %w", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(snap.Manifest, &manifest); err != nil {
		return nil, fmt.Errorf("plan: parse manifest: %w", err)
	}

	plan := &AgentSourcePlan{ToHash: snap.DirectoryHash}

	// Build the current-state index from existing source mappings.
	existingCaps := map[string]string{}    // source_key → content_hash
	caps, _ := h.Queries.ListSharedCapabilitiesBySource(ctx, sourceID)
	for _, c := range caps {
		existingCaps[c.SourceKey] = c.ContentHash
	}
	existingRoles := map[string]string{}   // source_role_id → last_import_hash
	roleMappings, _ := h.Queries.ListAgentSourceRolesBySource(ctx, sourceID)
	for _, r := range roleMappings {
		if r.LastImportHash.Valid {
			existingRoles[r.SourceRoleID] = r.LastImportHash.String
		}
	}
	existingSkills := map[string]string{}  // "roleID:skillID" → content_hash
	skillMappings, _ := h.Queries.ListAgentSourceSkillsBySource(ctx, sourceID)
	for _, s := range skillMappings {
		key := s.SourceRoleID + ":" + s.SourceSkillID
		if s.ContentHash.Valid {
			existingSkills[key] = s.ContentHash.String
		}
	}

	// Capabilities. M4: check version compatibility for updated capabilities
	// and flag affected roles as "blocked-by-update" when the new version
	// doesn't satisfy their binding's version requirement.
	if capsRaw, ok := manifest["capabilities"].([]any); ok {
		for _, cRaw := range capsRaw {
			c, ok := cRaw.(map[string]any)
			if !ok {
				continue
			}
			key, _ := c["id"].(string)
			name, _ := c["name"].(string)
			hash, _ := c["content_hash"].(string)
			version, _ := c["version"].(string)
			ch := AgentSourceChange{Key: key, Name: name}
			existingHash, hasCap := existingCaps[key]
			switch {
			case hasCap && existingHash == hash:
				ch.Action = "unchanged"
			case hasCap:
				ch.Action = "update"
				// M4: check if the version update breaks any consumer.
				blocked := h.checkCapabilityCompatibility(ctx, sourceID, key, version)
				if len(blocked) > 0 {
					ch.Reason = fmt.Sprintf("version updated; %d consuming role(s) may be blocked", len(blocked))
					for roleID, reason := range blocked {
						plan.BlockingIssues = append(plan.BlockingIssues, ScanDiagnostic{
							Severity: "error", Code: "capability_incompatible_update",
							Message: fmt.Sprintf("role %s: %s", roleID, reason),
						})
					}
				}
			default:
				ch.Action = "create"
			}
			plan.Capabilities = append(plan.Capabilities, ch)
		}
	}

	// M4: detect removed roles (present in source mappings but absent from
	// manifest). These are proposed as "archive-candidate", never silently
	// deleted. The UI shows the affected role and the admin decides.
	if rolesRaw, ok := manifest["roles"].([]any); ok {
		var manifestRoles []map[string]any
		for _, rRaw := range rolesRaw {
			if r, ok := rRaw.(map[string]any); ok {
				manifestRoles = append(manifestRoles, r)
			}
		}
		removedRoles := h.detectRemovedRoles(ctx, sourceID, manifestRoles)
		plan.Roles = append(plan.Roles, removedRoles...)
	}

	// Roles + skills + bindings + env + mcp (declarative scan of manifest).
	if rolesRaw, ok := manifest["roles"].([]any); ok {
		for _, rRaw := range rolesRaw {
			r, ok := rRaw.(map[string]any)
			if !ok {
				continue
			}
			roleID, _ := r["id"].(string)
			displayName, _ := r["display_name"].(string)
			rch := AgentSourceChange{Key: roleID, Name: displayName}
			if _, has := existingRoles[roleID]; has {
				rch.Action = "update"
			} else {
				rch.Action = "create"
			}
			plan.Roles = append(plan.Roles, rch)

			if skillsRaw, ok := r["skills"].([]any); ok {
				for _, sRaw := range skillsRaw {
					s, ok := sRaw.(map[string]any)
					if !ok {
						continue
					}
					sk, _ := s["id"].(string)
					sname, _ := s["name"].(string)
					shash, _ := s["content_hash"].(string)
					mapKey := roleID + ":" + sk
					sch := AgentSourceChange{Key: sk, Name: sname}
					existingHash, hasSkill := existingSkills[mapKey]
					switch {
					case hasSkill && existingHash == shash:
						sch.Action = "unchanged"
					case hasSkill:
						sch.Action = "update"
					default:
						sch.Action = "create"
					}
					plan.Skills = append(plan.Skills, sch)
				}
			}

			if bindingsRaw, ok := r["capability_bindings"].([]any); ok {
				for _, bRaw := range bindingsRaw {
					b, ok := bRaw.(map[string]any)
					if !ok {
						continue
					}
					bkey, _ := b["id"].(string)
					required, _ := b["required"].(bool)
					bch := AgentSourceChange{Key: bkey, Action: "add"}
					// Missing required capability = blocking issue.
					if required {
						if _, installed := existingCaps[bkey]; !installed {
							plan.BlockingIssues = append(plan.BlockingIssues, ScanDiagnostic{
								Severity: "error",
								Code:     "missing_required_capability",
								Message:  fmt.Sprintf("role %s requires capability %s which is not yet installed", roleID, bkey),
							})
							bch.Action = "blocked"
						}
					}
					plan.Bindings = append(plan.Bindings, bch)
				}
			}

			if envRaw, ok := r["env"].([]any); ok {
				for _, eRaw := range envRaw {
					e, ok := eRaw.(map[string]any)
					if !ok {
						continue
					}
					name, _ := e["name"].(string)
					required, _ := e["required"].(bool)
					configured, _ := e["configured"].(bool)
					if required && !configured {
						plan.Env.Missing = append(plan.Env.Missing, name)
						plan.BlockingIssues = append(plan.BlockingIssues, ScanDiagnostic{
							Severity: "error",
							Code:     "missing_required_env",
							Message:  fmt.Sprintf("role %s requires env %s which is not configured", roleID, name),
						})
					}
					plan.Env.Declared = append(plan.Env.Declared, name)
				}
			}

			if mcp, ok := r["mcp"].(map[string]any); ok {
				if unresolved, ok := mcp["unresolved_env"].([]any); ok && len(unresolved) > 0 {
					plan.BlockingIssues = append(plan.BlockingIssues, ScanDiagnostic{
						Severity: "error",
						Code:     "mcp_unresolved_env",
						Message:  fmt.Sprintf("role %s MCP references undeclared env keys", roleID),
					})
				}
			}
		}
	}

	// FromHash = the last applied snapshot's directory hash.
	if applied, err := h.Queries.GetLatestAppliedAgentSourceSnapshot(ctx, sourceID); err == nil {
		plan.FromHash = applied.DirectoryHash
	}
	return plan, nil
}

// --- HTTP endpoints ---

// GetAgentSourcePlan is the read-only diff between a snapshot and current state.
func (h *Handler) GetAgentSourcePlan(w http.ResponseWriter, r *http.Request) {
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
	// Optional ?snapshot= query; defaults to the latest preview snapshot.
	snapshotIDStr := r.URL.Query().Get("snapshot")
	var snapshotID pgtype.UUID
	if snapshotIDStr != "" {
		sid, err := parseUUIDOrBadRequest(w, snapshotIDStr, "snapshot")
		if !err {
			return
		}
		snapshotID = sid
	} else {
		snaps, err := h.Queries.ListAgentSourceSnapshots(r.Context(), src.ID)
		if err != nil || len(snaps) == 0 {
			writeError(w, http.StatusNotFound, "no snapshots available; scan first")
			return
		}
		snapshotID = snaps[0].ID
	}
	plan, err := h.BuildPlan(r.Context(), src.ID, snapshotID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build plan: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

// ApplyAgentSourceSnapshot applies one snapshot atomically. M2 apply requires
// the env apply token (the daemon's secret-bearing re-read result) to
// synchronize .env values; without it, apply proceeds but skips env-value sync
// and records the reason in the summary.
func (h *Handler) ApplyAgentSourceSnapshot(w http.ResponseWriter, r *http.Request) {
	if !agentWakerDirectorySyncEnabled(h, w, r) {
		return
	}
	src, ok := h.loadAgentSourceForMember(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireWorkspaceRole(w, r, uuidToString(src.WorkspaceID), "workspace not found", "owner", "admin"); !ok {
		return
	}
	var body struct {
		SnapshotID  string                 `json:"snapshot_id"`
		EnvMergeMode string                `json:"env_merge_mode"`
		EnvValues    map[string]map[string]string `json:"env_values"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	snapshotID, ok := parseUUIDOrBadRequest(w, body.SnapshotID, "snapshot_id")
	if !ok {
		return
	}
	if body.EnvMergeMode == "" {
		body.EnvMergeMode = "source-authoritative"
	}
	if body.EnvMergeMode != "source-authoritative" && body.EnvMergeMode != "merge-preserve" {
		writeError(w, http.StatusBadRequest, "env_merge_mode must be source-authoritative or merge-preserve")
		return
	}
	// The env values travel in the explicit apply payload (owner/admin only,
	// TLS). The server does NOT read them from the snapshot (snapshots are
	// value-free). Defense-in-depth: confirm no value leaks into logs.
	result, err := h.ApplySnapshot(r.Context(), ApplySnapshotInput{
		SourceID:     src.ID,
		SnapshotID:   snapshotID,
		WorkspaceID:  src.WorkspaceID,
		EnvMergeMode: body.EnvMergeMode,
		EnvValues:    body.EnvValues,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "apply failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// RollbackAgentSource re-applies a prior superseded snapshot, making it the
// active applied snapshot again. It uses the same atomic apply path; the prior
// snapshot's manifest is already stored value-free, so rollback is a re-apply
// of that manifest. Env values are NOT re-synchronized on rollback (they would
// require a fresh daemon re-read); the encrypted env column is left as-is so
// the agent keeps its last-known-good configuration.
func (h *Handler) RollbackAgentSource(w http.ResponseWriter, r *http.Request) {
	if !agentWakerDirectorySyncEnabled(h, w, r) {
		return
	}
	src, ok := h.loadAgentSourceForMember(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireWorkspaceRole(w, r, uuidToString(src.WorkspaceID), "workspace not found", "owner", "admin"); !ok {
		return
	}
	var body struct {
		SnapshotID string `json:"snapshot_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	snapshotID, ok := parseUUIDOrBadRequest(w, body.SnapshotID, "snapshot_id")
	if !ok {
		return
	}
	// The target snapshot must exist and belong to this source; it must be a
	// prior 'applied' or 'superseded' snapshot (rollback target).
	target, err := h.Queries.GetAgentSourceSnapshotInSource(r.Context(), db.GetAgentSourceSnapshotInSourceParams{
		ID: snapshotID, SourceID: src.ID,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "rollback target snapshot not found")
		return
	}
	if target.Status != "superseded" && target.Status != "applied" {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("can only rollback to an applied or superseded snapshot, got %s", target.Status))
		return
	}
	// Re-apply the target manifest with no env values (env is preserved as-is).
	// Flip the target back to 'preview' so ApplySnapshot accepts it, then apply.
	if err := h.Queries.MarkAgentSourceSnapshotFailed(r.Context(), target.ID); err != nil {
		// MarkAgentSourceSnapshotFailed sets status='failed' which ApplySnapshot
		// accepts as a re-applyable state (preview|failed). This is the rollback
		// entry point.
		writeError(w, http.StatusInternalServerError, "failed to stage rollback: "+err.Error())
		return
	}
	result, err := h.ApplySnapshot(r.Context(), ApplySnapshotInput{
		SourceID:    src.ID,
		SnapshotID:  target.ID,
		WorkspaceID: src.WorkspaceID,
		// No EnvValues on rollback: the encrypted env column is preserved.
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "rollback failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// unused import guard: keep chi in scope for future sub-route params.
var _ = chi.URLParam
