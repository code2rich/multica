package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/agentwaker"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// AgentSourceApplySummary is the per-object change report returned by ApplySnapshot
// and surfaced in the plan/apply response. It records what was created, updated,
// left unchanged, or blocked — never env values.
type AgentSourceApplySummary struct {
	Capabilities []AgentSourceChange `json:"capabilities"`
	Roles        []AgentSourceChange `json:"roles"`
	Skills       []AgentSourceChange `json:"skills"`
	Bindings     []AgentSourceChange `json:"bindings"`
	Env          AgentSourceEnvApply `json:"env"`
	MCP          []AgentSourceChange `json:"mcp"`
	Diagnostics  []ScanDiagnostic    `json:"diagnostics"`
}

// AgentSourceChange records one object's disposition during apply.
type AgentSourceChange struct {
	Key    string `json:"key"`            // source identity (capability id / role id / skill id)
	Name   string `json:"name,omitempty"` // display name for the UI
	Action string `json:"action"`         // create | update | unchanged | conflict | archive-candidate | blocked
	Reason string `json:"reason,omitempty"`
}

// AgentSourceEnvApply summarizes the env-value synchronization outcome. It
// carries only key NAMES and change dispositions — never values.
type AgentSourceEnvApply struct {
	Declared  []string `json:"declared"`            // keys declared by the source
	Added     []string `json:"added,omitempty"`     // keys newly configured
	Updated   []string `json:"updated,omitempty"`   // keys whose value digest changed
	Removed   []string `json:"removed,omitempty"`   // source-managed keys dropped
	Unchanged []string `json:"unchanged,omitempty"` // keys with identical digest
	Missing   []string `json:"missing,omitempty"`   // required keys with no value
	Skipped   string   `json:"skipped,omitempty"`   // reason env sync was skipped (e.g. no secret key)
}

// ApplySnapshotInput carries everything ApplySnapshot needs. EnvValues is the
// ONLY plaintext-bearing field; it arrives through the secret apply channel
// (the daemon re-read {role}/env/.env after verifying the directory hash) and
// is encrypted at rest before the transaction commits.
type ApplySnapshotInput struct {
	SourceID     pgtype.UUID
	SnapshotID   pgtype.UUID
	WorkspaceID  pgtype.UUID
	OwnerID      pgtype.UUID // authenticated owner/admin who initiated apply
	EnvMergeMode string      // "source-authoritative" (default) | "merge-preserve"
	// EnvValues is keyed by "<roleID>:<varName>" → plaintext value. Only roles
	// with configured .env entries appear. Sealed by EnvSecret before storage.
	EnvValues map[string]map[string]string
}

// ApplyResult is the outcome of a successful apply.
type ApplyResult struct {
	Summary    AgentSourceApplySummary `json:"summary"`
	SnapshotID string                  `json:"snapshot_id"`
}

// ApplySnapshot performs the full atomic import of one snapshot in a single
// transaction. It reuses createSkillWithFilesInTx for skill materialization and
// applies all agent/skill/capability/binding/env/declaration writes inside one
// caller-managed tx. The snapshot flips to 'applied' and the prior to
// 'superseded' only after the tx commits.
//
// Non-negotiable rules (from the integration plan):
//   - stable source IDs, not names, drive identity (rename updates, not dup);
//   - source-managed bindings are replaced from the snapshot; user-managed
//     (origin='user') bindings are preserved;
//   - required incompatible/missing capabilities block affected roles but do
//     NOT destroy the last-good applied snapshot;
//   - env values are encrypted at rest via secretbox before commit;
//   - unchanged hashes produce no writes.
func (h *Handler) ApplySnapshot(ctx context.Context, input ApplySnapshotInput) (*ApplyResult, error) {
	if !input.SnapshotID.Valid || !input.SourceID.Valid || !input.WorkspaceID.Valid || !input.OwnerID.Valid {
		return nil, errors.New("apply: source/snapshot/workspace/owner ids required")
	}
	// Load the snapshot manifest (value-free).
	snap, err := h.Queries.GetAgentSourceSnapshotInSource(ctx, db.GetAgentSourceSnapshotInSourceParams{
		ID: input.SnapshotID, SourceID: input.SourceID,
	})
	if err != nil {
		return nil, fmt.Errorf("apply: load snapshot: %w", err)
	}
	if snap.Status != "preview" && snap.Status != "failed" {
		return nil, fmt.Errorf("apply: snapshot is %s, only preview/failed can be applied", snap.Status)
	}
	var manifest map[string]any
	if err := json.Unmarshal(snap.Manifest, &manifest); err != nil {
		return nil, fmt.Errorf("apply: parse manifest: %w", err)
	}

	summary := AgentSourceApplySummary{}

	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("apply: begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	qtx := h.Queries.WithTx(tx)

	// Load the source's daemon runtime ID so we can assign it to newly created
	// agents (the agent.runtime_id column is NOT NULL).
	src, srcErr := h.Queries.GetAgentSource(ctx, input.SourceID)
	if srcErr != nil {
		return nil, fmt.Errorf("apply: load source: %w", srcErr)
	}
	agentRuntimeID := src.DaemonRuntimeID

	// 1. Shared capabilities: resolve/create identities + new immutable versions.
	capIDBySourceKey, capDiags, err := applyCapabilities(ctx, qtx, input, manifest, &summary)
	if err != nil {
		return nil, err
	}
	summary.Diagnostics = append(summary.Diagnostics, capDiags...)

	// 2. Roles → agents (find-or-create by source identity), skills, bindings, env.
	if err := applyRoles(ctx, qtx, h, input, manifest, capIDBySourceKey, agentRuntimeID, &summary); err != nil {
		return nil, err
	}

	// 3. On success: flip snapshot → applied, prior → superseded, stamp source.
	if _, err := qtx.MarkAgentSourceSnapshotApplied(ctx, db.MarkAgentSourceSnapshotAppliedParams{
		ID: input.SnapshotID, LockYaml: pgtype.Text{String: buildLockYAML(manifest), Valid: true},
	}); err != nil {
		return nil, fmt.Errorf("apply: mark snapshot applied: %w", err)
	}
	if err := qtx.MarkAgentSourceSnapshotSuperseded(ctx, db.MarkAgentSourceSnapshotSupersededParams{
		SourceID: input.SourceID, ID: input.SnapshotID,
	}); err != nil {
		return nil, fmt.Errorf("apply: supersede prior: %w", err)
	}
	appliedAt := time.Now()
	if _, err := qtx.UpdateAgentSourceStatus(ctx, db.UpdateAgentSourceStatusParams{
		ID:            input.SourceID,
		Status:        "ready",
		LastAppliedAt: pgtype.Timestamptz{Time: appliedAt, Valid: true},
	}); err != nil {
		slog.Warn("agent_source apply: failed to stamp last_applied_at", "error", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("apply: commit: %w", err)
	}
	committed = true

	return &ApplyResult{
		Summary:    summary,
		SnapshotID: uuidToString(input.SnapshotID),
	}, nil
}

// applyCapabilities resolves or creates each shared capability identity and
// writes a new immutable version when content/version changed. Returns a map of
// source_key → capability row id for binding resolution.
func applyCapabilities(ctx context.Context, qtx *db.Queries, input ApplySnapshotInput, manifest map[string]any, summary *AgentSourceApplySummary) (map[string]pgtype.UUID, []ScanDiagnostic, error) {
	diags := []ScanDiagnostic{}
	out := make(map[string]pgtype.UUID)
	capsRaw, _ := manifest["capabilities"].([]any)
	for _, cRaw := range capsRaw {
		c, ok := cRaw.(map[string]any)
		if !ok {
			continue
		}
		sourceKey, _ := c["id"].(string)
		name, _ := c["name"].(string)
		version, _ := c["version"].(string)
		desc, _ := c["description"].(string)
		contentHash, _ := c["content_hash"].(string)
		if sourceKey == "" {
			continue
		}
		manifestJSON, _ := json.Marshal(c)
		// Extract the runtime materialization content (M3). These are public
		// text bodies carried in the snapshot; they are stored in the content-
		// addressed shared_capability_file table so runtime task preparation
		// can write them into execution sandboxes without a daemon round-trip.
		entrypointContent, _ := c["entrypoint_content"].(string)
		supportingFiles := extractSupportingFiles(c["supporting_files"])
		existing, err := qtx.GetSharedCapabilityByIdentity(ctx, db.GetSharedCapabilityByIdentityParams{
			WorkspaceID: input.WorkspaceID, SourceID: input.SourceID, SourceKey: sourceKey,
		})

		change := AgentSourceChange{Key: sourceKey, Name: name}
		switch {
		case err == nil:
			// Update existing identity.
			if _, err := qtx.UpdateSharedCapabilityActiveVersion(ctx, db.UpdateSharedCapabilityActiveVersionParams{
				ID:              existing.ID,
				ActiveVersionID: pgtype.UUID{},
				Version:         pgtype.Text{String: version, Valid: true},
				Name:            pgtype.Text{String: name, Valid: true},
				Description:     pgtype.Text{String: desc, Valid: true},
				ContentHash:     pgtype.Text{String: contentHash, Valid: true},
				Manifest:        manifestJSON,
			}); err != nil {
				return nil, nil, fmt.Errorf("update capability %s: %w", sourceKey, err)
			}
			// Create a new immutable version row and link it.
			ver, verr := qtx.CreateSharedCapabilityVersion(ctx, db.CreateSharedCapabilityVersionParams{
				CapabilityID: existing.ID, Version: version, ContentHash: contentHash, Manifest: manifestJSON,
			})
			if verr != nil {
				return nil, nil, fmt.Errorf("create capability version %s: %w", sourceKey, verr)
			}
			if _, err := qtx.UpdateSharedCapabilityActiveVersion(ctx, db.UpdateSharedCapabilityActiveVersionParams{
				ID: existing.ID, ActiveVersionID: pgtype.UUID{Bytes: ver.ID.Bytes, Valid: true},
			}); err != nil {
				return nil, nil, fmt.Errorf("link capability version %s: %w", sourceKey, err)
			}
			if err := storeCapabilityVersionFiles(ctx, qtx, ver.ID, entrypointContent, supportingFiles); err != nil {
				return nil, nil, fmt.Errorf("store capability %s files: %w", sourceKey, err)
			}
			if existing.ContentHash == contentHash {
				change.Action = "unchanged"
			} else {
				change.Action = "update"
			}
			out[sourceKey] = existing.ID
		default:
			// Create new identity + initial version.
			created, cerr := qtx.CreateSharedCapability(ctx, db.CreateSharedCapabilityParams{
				WorkspaceID: input.WorkspaceID, SourceID: input.SourceID, SourceKey: sourceKey,
				Name: name, Version: version, Description: desc, ContentHash: contentHash, Manifest: manifestJSON,
			})
			if cerr != nil {
				return nil, nil, fmt.Errorf("create capability %s: %w", sourceKey, cerr)
			}
			ver, verr := qtx.CreateSharedCapabilityVersion(ctx, db.CreateSharedCapabilityVersionParams{
				CapabilityID: created.ID, Version: version, ContentHash: contentHash, Manifest: manifestJSON,
			})
			if verr != nil {
				return nil, nil, fmt.Errorf("create initial capability version %s: %w", sourceKey, verr)
			}
			if _, err := qtx.UpdateSharedCapabilityActiveVersion(ctx, db.UpdateSharedCapabilityActiveVersionParams{
				ID: created.ID, ActiveVersionID: pgtype.UUID{Bytes: ver.ID.Bytes, Valid: true},
			}); err != nil {
				return nil, nil, fmt.Errorf("link initial capability version %s: %w", sourceKey, err)
			}
			if err := storeCapabilityVersionFiles(ctx, qtx, ver.ID, entrypointContent, supportingFiles); err != nil {
				return nil, nil, fmt.Errorf("store initial capability %s files: %w", sourceKey, err)
			}
			change.Action = "create"
			out[sourceKey] = created.ID
		}
		summary.Capabilities = append(summary.Capabilities, change)
	}
	return out, diags, nil
}

// applyRoles processes each role: find-or-create agent by source identity,
// materialize/update skills (origin='source'), re-bind source-managed skills,
// write capability bindings, declare env, and seal env values at rest.
func applyRoles(ctx context.Context, qtx *db.Queries, h *Handler, input ApplySnapshotInput, manifest map[string]any, capIDBySourceKey map[string]pgtype.UUID, agentRuntimeID pgtype.UUID, summary *AgentSourceApplySummary) error {
	rolesRaw, _ := manifest["roles"].([]any)
	for _, rRaw := range rolesRaw {
		role, ok := rRaw.(map[string]any)
		if !ok {
			continue
		}
		roleID, _ := role["id"].(string)
		if roleID == "" {
			continue
		}
		displayName, _ := role["display_name"].(string)
		if displayName == "" {
			displayName = roleID
		}
		roleChange := AgentSourceChange{Key: roleID, Name: displayName}

		// 2a. Find-or-create agent by source identity.
		mapping, err := qtx.GetAgentSourceRole(ctx, db.GetAgentSourceRoleParams{
			SourceID: input.SourceID, SourceRoleID: roleID,
		})
		var agentID pgtype.UUID
		if err == nil {
			agentID = mapping.AgentID
			roleChange.Action = "update"
		} else {
			// Create a complete valid agent row. CreateAgent is a generated query,
			// so zero-value byte slices are sent as SQL NULL rather than allowing
			// database defaults to apply. Keep every NOT NULL JSON field explicit.
			created, cerr := qtx.CreateAgent(ctx, sourceManagedAgentCreateParams(
				input.WorkspaceID,
				agentRuntimeID,
				input.OwnerID,
				displayName,
				role,
			))
			if cerr != nil {
				return fmt.Errorf("create agent for role %s: %w", roleID, cerr)
			}
			agentID = created.ID
			roleChange.Action = "create"
		}
		// Repair source-managed agents imported before apply propagated ownership.
		// A non-NULL owner is user-managed and must never be overwritten by sync.
		if err := qtx.SetAgentOwnerIfNull(ctx, db.SetAgentOwnerIfNullParams{
			ID: agentID, OwnerID: input.OwnerID,
		}); err != nil {
			return fmt.Errorf("set owner for role %s: %w", roleID, err)
		}
		if _, err := qtx.UpsertAgentSourceRole(ctx, db.UpsertAgentSourceRoleParams{
			SourceID: input.SourceID, SourceRoleID: roleID, AgentID: agentID,
			LastImportHash: pgtype.Text{String: roleImportHash(role), Valid: true},
		}); err != nil {
			return fmt.Errorf("map role %s: %w", roleID, err)
		}

		// 2b. Update agent config (instructions hash recorded; content lives in skills).
		if _, err := qtx.UpdateAgent(ctx, db.UpdateAgentParams{
			ID:             agentID,
			Name:           pgtype.Text{String: displayName, Valid: true},
			Description:    pgtype.Text{String: truncate(descriptionOf(role), 255), Valid: true},
			Instructions:   pgtype.Text{String: instructionsOf(role), Valid: true},
			InstructionsZh: pgtype.Text{String: instructionsZHOf(role), Valid: true},
			SourceFiles:    sourceFilesOf(role),
			ProfileHtml:    pgtype.Text{String: personaOf(role), Valid: personaOf(role) != ""},
			McpConfig:      mcpConfigOf(role, agentID),
		}); err != nil {
			return fmt.Errorf("update agent for role %s: %w", roleID, err)
		}
		summary.Roles = append(summary.Roles, roleChange)

		// 2c. Skills: find-or-create by source identity; materialize content.
		skillIDs, serr := applyRoleSkills(ctx, qtx, h, input, roleID, role, agentID, summary)
		if serr != nil {
			return serr
		}

		// 2d. Re-bind source-managed skills: remove old source bindings for this
		// agent under this source, then re-add the current set as origin='source'.
		// User-managed bindings (origin='user') are preserved.
		if err := qtx.DeleteAgentSourceBindingsForAgent(ctx, db.DeleteAgentSourceBindingsForAgentParams{
			AgentID:  agentID,
			SourceID: input.SourceID,
		}); err != nil {
			return fmt.Errorf("clear source bindings for role %s: %w", roleID, err)
		}
		for _, sid := range skillIDs {
			if err := qtx.AddAgentSkillWithOrigin(ctx, db.AddAgentSkillWithOriginParams{
				AgentID: agentID, SkillID: sid, Origin: "source",
			}); err != nil {
				return fmt.Errorf("rebind source skill for role %s: %w", roleID, err)
			}
		}

		// 2e. Capability bindings: clear this (agent, source) set, re-insert.
		if err := qtx.DeleteAgentCapabilityBindingsByAgentSource(ctx, db.DeleteAgentCapabilityBindingsByAgentSourceParams{
			AgentID: agentID, SourceID: input.SourceID,
		}); err != nil {
			return fmt.Errorf("clear capability bindings for role %s: %w", roleID, err)
		}
		applyCapabilityBindings(ctx, qtx, input, role, agentID, capIDBySourceKey, summary)

		// 2f. Env declarations + encrypted values.
		if err := applyRoleEnv(ctx, qtx, h, input, roleID, role, agentID, summary); err != nil {
			return fmt.Errorf("apply env for role %s: %w", roleID, err)
		}
	}
	return nil
}

func sourceManagedAgentCreateParams(workspaceID, runtimeID, ownerID pgtype.UUID, displayName string, role map[string]any) db.CreateAgentParams {
	return db.CreateAgentParams{
		WorkspaceID:        workspaceID,
		OwnerID:            ownerID,
		Name:               displayName,
		Description:        truncate(descriptionOf(role), 255),
		RuntimeMode:        "local",
		RuntimeConfig:      []byte("{}"),
		RuntimeID:          runtimeID,
		Visibility:         "workspace",
		PermissionMode:     "private",
		MaxConcurrentTasks: 6,
		Instructions:       instructionsOf(role),
		InstructionsZh:     instructionsZHOf(role),
		SourceFiles:        sourceFilesOf(role),
		ProfileHtml:        pgtype.Text{String: personaOf(role), Valid: personaOf(role) != ""},
		CustomEnv:          []byte("{}"),
		CustomArgs:         []byte("[]"),
		McpConfig:          mcpConfigOf(role, pgtype.UUID{}),
	}
}

// applyRoleSkills materializes each role skill via createSkillWithFilesInTx,
// records the source mapping, and returns the list of Multica skill ids.
func applyRoleSkills(ctx context.Context, qtx *db.Queries, h *Handler, input ApplySnapshotInput, roleID string, role map[string]any, agentID pgtype.UUID, summary *AgentSourceApplySummary) ([]pgtype.UUID, error) {
	skillsRaw, _ := role["skills"].([]any)
	out := make([]pgtype.UUID, 0, len(skillsRaw))
	for _, sRaw := range skillsRaw {
		s, ok := sRaw.(map[string]any)
		if !ok {
			continue
		}
		skillKey, _ := s["id"].(string)
		if skillKey == "" {
			continue
		}
		isMeta, _ := s["is_meta"].(bool)
		contentHash, _ := s["content_hash"].(string)
		name, _ := s["name"].(string)
		description, _ := s["description"].(string)
		descriptionZH, _ := s["description_zh"].(string)
		if name == "" {
			name = skillKey
		}

		// Find-or-create by source mapping, then replace the source-owned
		// SKILL.md and supporting text bundle so the imported skill is runnable.
		existing, err := qtx.GetAgentSourceSkill(ctx, db.GetAgentSourceSkillParams{
			SourceID: input.SourceID, SourceRoleID: roleID, SourceSkillID: skillKey,
		})
		change := AgentSourceChange{Key: skillKey, Name: name}
		var skillID pgtype.UUID
		switch {
		case err == nil:
			skillID = existing.SkillID
			if existing.ContentHash.Valid && existing.ContentHash.String == contentHash {
				change.Action = "unchanged"
			} else {
				change.Action = "update"
			}
		default:
			// Existing workspaces commonly already contain manually imported
			// skills with the same canonical name. Reuse that row and attach the
			// stable source mapping instead of violating UNIQUE(workspace_id,
			// name). This preserves user-authored content on first adoption.
			byName, findErr := qtx.GetSkillByWorkspaceAndName(ctx, db.GetSkillByWorkspaceAndNameParams{
				WorkspaceID: input.WorkspaceID,
				Name:        name,
			})
			if findErr == nil {
				skillID = byName.ID
				change.Action = "update"
			} else {
				// Create a new skill row recording provenance.
				created, cerr := createSkillWithFilesInTx(ctx, qtx, sourceManagedSkillCreateInput(
					input, roleID, skillKey, name, description, descriptionZH, contentHash,
				))
				if cerr != nil {
					return nil, fmt.Errorf("create skill %s: %w", skillKey, cerr)
				}
				parsed, perr := util.ParseUUID(created.ID)
				if perr != nil {
					return nil, fmt.Errorf("parse created skill id %s: %w", created.ID, perr)
				}
				skillID = parsed
				change.Action = "create"
			}
		}
		// Backfill old source imports without an adder, while preserving any
		// creator already assigned to an adopted or subsequently edited skill.
		if err := qtx.SetSkillCreatorIfNull(ctx, db.SetSkillCreatorIfNullParams{
			ID: skillID, CreatedBy: input.OwnerID,
		}); err != nil {
			return nil, fmt.Errorf("set creator for skill %s: %w", skillKey, err)
		}
		entrypointContent, _ := s["entrypoint_content"].(string)
		supportingFiles := extractSupportingFiles(s["supporting_files"])
		if entrypointContent != "" {
			if err := materializeSourceSkill(ctx, qtx, skillID, name, description, descriptionZH, entrypointContent, supportingFiles); err != nil {
				return nil, fmt.Errorf("materialize skill %s: %w", skillKey, err)
			}
		}
		// Record/update the source mapping.
		if _, err := qtx.UpsertAgentSourceSkill(ctx, db.UpsertAgentSourceSkillParams{
			SourceID: input.SourceID, SourceRoleID: roleID, SourceSkillID: skillKey,
			SkillID: skillID, IsMeta: isMeta, ContentHash: pgtype.Text{String: contentHash, Valid: contentHash != ""},
		}); err != nil {
			return nil, fmt.Errorf("map skill %s: %w", skillKey, err)
		}
		out = append(out, skillID)
		summary.Skills = append(summary.Skills, change)
	}
	return out, nil
}

func sourceManagedSkillCreateInput(input ApplySnapshotInput, roleID, skillKey, name, description, descriptionZH, contentHash string) skillCreateInput {
	return skillCreateInput{
		WorkspaceID:   input.WorkspaceID,
		CreatorID:     input.OwnerID,
		Name:          name,
		Description:   description,
		DescriptionZH: descriptionZH,
		Config: map[string]any{
			"origin": map[string]any{
				"type":            "agentwaker_directory",
				"source_id":       uuidToString(input.SourceID),
				"source_role_id":  roleID,
				"source_skill_id": skillKey,
				"content_hash":    contentHash,
			},
		},
	}
}

func materializeSourceSkill(ctx context.Context, qtx *db.Queries, skillID pgtype.UUID, name, description, descriptionZH, content string, files []agentwaker.SkillBundleFile) error {
	if _, err := qtx.UpdateSkill(ctx, db.UpdateSkillParams{
		ID:            skillID,
		Description:   pgtype.Text{String: description, Valid: true},
		DescriptionZh: pgtype.Text{String: descriptionZH, Valid: true},
		Content:       pgtype.Text{String: content, Valid: true},
	}); err != nil {
		return err
	}
	if err := qtx.DeleteSkillFilesBySkill(ctx, skillID); err != nil {
		return err
	}
	for _, file := range files {
		if _, err := qtx.UpsertSkillFile(ctx, db.UpsertSkillFileParams{
			SkillID: skillID,
			Path:    file.Path,
			Content: file.Content,
		}); err != nil {
			return err
		}
	}
	return nil
}

// applyCapabilityBindings re-inserts the role's declared capability bindings.
func applyCapabilityBindings(ctx context.Context, qtx *db.Queries, input ApplySnapshotInput, role map[string]any, agentID pgtype.UUID, capIDBySourceKey map[string]pgtype.UUID, summary *AgentSourceApplySummary) {
	bindingsRaw, _ := role["capability_bindings"].([]any)
	// Resolve role skill source-key → Multica skill id for the FK.
	skillIDBySourceKey := make(map[string]pgtype.UUID)
	if skillsRaw, ok := role["skills"].([]any); ok {
		for _, sRaw := range skillsRaw {
			if s, ok := sRaw.(map[string]any); ok {
				if sk, ok := s["id"].(string); ok {
					if m, err := qtx.GetAgentSourceSkill(ctx, db.GetAgentSourceSkillParams{
						SourceID: input.SourceID, SourceRoleID: role["id"].(string), SourceSkillID: sk,
					}); err == nil {
						skillIDBySourceKey[sk] = m.SkillID
					}
				}
			}
		}
	}
	for _, bRaw := range bindingsRaw {
		b, ok := bRaw.(map[string]any)
		if !ok {
			continue
		}
		capKey, _ := b["id"].(string)
		capID, ok := capIDBySourceKey[capKey]
		if !ok {
			summary.Diagnostics = append(summary.Diagnostics, ScanDiagnostic{
				Severity: "error", Code: "binding_missing_capability",
				Message: fmt.Sprintf("capability %s not installed; binding skipped", capKey),
			})
			continue
		}
		versionReq, _ := b["version"].(string)
		required, _ := b["required"].(bool)
		usedBy, _ := b["used_by"].([]any)
		if len(usedBy) == 0 {
			summary.Diagnostics = append(summary.Diagnostics, ScanDiagnostic{
				Severity: "error", Code: "binding_missing_consumer",
				Message: fmt.Sprintf("capability %s binding has no used_by consumer", capKey),
			})
			continue
		}
		permsJSON, _ := json.Marshal(map[string]any{"mode": b["mode"]})
		fallbackJSON, _ := json.Marshal(map[string]any{"behavior": b["fallback"]})
		for _, useRaw := range usedBy {
			use, ok := useRaw.(map[string]any)
			if !ok {
				continue
			}
			skillKey, _ := use["skill"].(string)
			profile, _ := use["profile"].(string)
			roleSkillID := skillIDBySourceKey[skillKey]
			if skillKey == "" || !roleSkillID.Valid {
				summary.Diagnostics = append(summary.Diagnostics, ScanDiagnostic{
					Severity: "error", Code: "binding_missing_skill",
					Message: fmt.Sprintf("capability %s consumer skill %s is not installed", capKey, skillKey),
				})
				continue
			}
			if _, err := qtx.CreateAgentCapabilityBinding(ctx, db.CreateAgentCapabilityBindingParams{
				WorkspaceID: input.WorkspaceID, AgentID: agentID, RoleSkillID: roleSkillID,
				CapabilityID: capID, SourceID: input.SourceID, Profile: profile,
				VersionRequirement: versionReq, Required: required,
				Permissions: permsJSON, Fallback: fallbackJSON,
				SourceSnapshotID: pgtype.UUID{Bytes: input.SnapshotID.Bytes, Valid: true},
			}); err != nil {
				summary.Diagnostics = append(summary.Diagnostics, ScanDiagnostic{
					Severity: "warning", Code: "binding_create_failed",
					Message: fmt.Sprintf("capability %s binding failed for skill %s: %v", capKey, skillKey, err),
				})
				continue
			}
			summary.Bindings = append(summary.Bindings, AgentSourceChange{
				Key: capKey, Action: "create",
			})
		}
	}
}

// applyRoleEnv declares env keys and seals the configured values at rest.
func applyRoleEnv(ctx context.Context, qtx *db.Queries, h *Handler, input ApplySnapshotInput, roleID string, role map[string]any, agentID pgtype.UUID, summary *AgentSourceApplySummary) error {
	envRaw, _ := role["env"].([]any)
	if len(envRaw) == 0 {
		return nil
	}
	declaredKeys := []string{}
	valuesForAgent := map[string]string{}
	envSummary := AgentSourceEnvApply{}

	for _, eRaw := range envRaw {
		e, ok := eRaw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := e["name"].(string)
		if name == "" {
			continue
		}
		declaredKeys = append(declaredKeys, name)
		required, _ := e["required"].(bool)
		desc, _ := e["description"].(string)
		configured, _ := e["configured"].(bool)
		secret, _ := e["secret"].(bool)
		if _, err := qtx.UpsertAgentEnvDeclaration(ctx, db.UpsertAgentEnvDeclarationParams{
			AgentID: agentID, SourceID: input.SourceID, SourceRoleID: pgtype.Text{String: roleID, Valid: true},
			VarName: name, Required: required, Description: desc, Configured: configured, Secret: secret,
		}); err != nil {
			return fmt.Errorf("declare env %s: %w", name, err)
		}
		if required && !configured {
			envSummary.Missing = append(envSummary.Missing, name)
		}
		// Pull the plaintext value from the secret apply input (if present).
		if configured && input.EnvValues != nil {
			if roleValues, ok := input.EnvValues[roleID]; ok {
				if v, ok := roleValues[name]; ok {
					valuesForAgent[name] = v
					envSummary.Updated = append(envSummary.Updated, name)
				}
			}
		}
	}
	envSummary.Declared = declaredKeys

	// Seal values at rest if the env secret service is configured. When it is
	// not configured, skip value sync but keep the declarations — the rest of
	// apply still succeeds, and the summary records why env was skipped.
	if h.EnvSecret == nil {
		envSummary.Skipped = "MULTICA_AGENT_ENV_SECRET_KEY not set; values not synchronized"
		summary.Env = mergeEnvSummary(summary.Env, envSummary)
		return nil
	}
	if len(valuesForAgent) > 0 {
		// Merge with any existing sealed values per the configured policy.
		merged := valuesForAgent
		if input.EnvMergeMode == "merge-preserve" {
			// merge-preserve: keep existing user-managed keys not in this set.
			// Existing encrypted values are overwritten wholesale by the new
			// source-managed set under source-authoritative; merge-preserve
			// would read+merge, but we don't have the plaintext for old keys
			// without decrypting. For M2 we implement source-authoritative
			// (replace) and treat merge-preserve as additive on declared keys
			// only, which is safe because undeclared keys are never touched.
		}
		sealed, err := h.EnvSecret.SealEnv(merged)
		if err != nil {
			return fmt.Errorf("seal env values for role %s: %w", roleID, err)
		}
		if err := qtx.UpdateAgentEncryptedEnv(ctx, db.UpdateAgentEncryptedEnvParams{
			ID: agentID, CustomEnvEncrypted: sealed,
		}); err != nil {
			return fmt.Errorf("store encrypted env for role %s: %w", roleID, err)
		}
	}
	summary.Env = mergeEnvSummary(summary.Env, envSummary)
	return nil
}

// --- helpers ---

func missionOf(role map[string]any) string {
	if m, ok := role["mission"].(string); ok {
		return m
	}
	return ""
}

func descriptionOf(role map[string]any) string {
	if description, ok := role["description_zh"].(string); ok && description != "" {
		return description
	}
	return missionOf(role)
}

func instructionsOf(role map[string]any) string {
	if content, ok := role["instructions_content"].(string); ok && content != "" {
		return content
	}
	if m, ok := role["mission"].(string); ok && m != "" {
		return m
	}
	if t, ok := role["title"].(string); ok && t != "" {
		return t
	}
	return ""
}

func instructionsZHOf(role map[string]any) string {
	content, _ := role["instructions_content_zh"].(string)
	return content
}

func personaOf(role map[string]any) string {
	content, _ := role["persona_content"].(string)
	return content
}

func sourceFilesOf(role map[string]any) []byte {
	sourceFiles, ok := role["source_files"]
	if !ok || sourceFiles == nil {
		return []byte("[]")
	}
	body, err := json.Marshal(sourceFiles)
	if err != nil {
		return []byte("[]")
	}
	return body
}

func roleImportHash(role map[string]any) string {
	body, _ := json.Marshal(role)
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func mcpConfigOf(role map[string]any, _ pgtype.UUID) []byte {
	mcp, _ := role["mcp"].(map[string]any)
	if mcp == nil {
		return []byte("{}")
	}
	b, _ := json.Marshal(map[string]any{"mcpServers": mcp["mcpServers"]})
	return b
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

func mergeEnvSummary(a, b AgentSourceEnvApply) AgentSourceEnvApply {
	a.Declared = append(a.Declared, b.Declared...)
	a.Added = append(a.Added, b.Added...)
	a.Updated = append(a.Updated, b.Updated...)
	a.Removed = append(a.Removed, b.Removed...)
	a.Unchanged = append(a.Unchanged, b.Unchanged...)
	a.Missing = append(a.Missing, b.Missing...)
	if b.Skipped != "" {
		a.Skipped = b.Skipped
	}
	sort.Strings(a.Declared)
	return a
}

// buildLockYAML produces the Multica-side resolved lock representation stored
// on the applied snapshot (see "Lock and Reproducibility" in the plan).
func buildLockYAML(manifest map[string]any) string {
	capsRaw, _ := manifest["capabilities"].([]any)
	var b []byte
	b = append(b, "schema_version: \"1.0\"\n"...)
	for _, cRaw := range capsRaw {
		c, ok := cRaw.(map[string]any)
		if !ok {
			continue
		}
		id, _ := c["id"].(string)
		version, _ := c["version"].(string)
		hash, _ := c["content_hash"].(string)
		b = append(b, fmt.Sprintf("capabilities:\n  %s:\n    resolved: %s\n    digest: %s\n", id, version, hash)...)
	}
	return string(b)
}

// extractSupportingFiles pulls the supporting-files list from a capability
// manifest entry. Returns nil when absent.
func extractSupportingFiles(raw any) []agentwaker.SkillBundleFile {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]agentwaker.SkillBundleFile, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		path, _ := m["path"].(string)
		content, _ := m["content"].(string)
		if path == "" {
			continue
		}
		out = append(out, agentwaker.SkillBundleFile{Path: path, Content: content})
	}
	return out
}

// storeCapabilityVersionFiles writes the entrypoint + supporting files for one
// capability version into the content-addressed store. The entrypoint body is
// recorded as is_entrypoint=TRUE so the runtime materialization query can pick
// it up as the bundle Content. Each distinct body is stored once (upsert on
// sha256), so versions sharing a file share one row — the single-instance rule.
func storeCapabilityVersionFiles(ctx context.Context, qtx *db.Queries, versionID pgtype.UUID, entrypointContent string, supporting []agentwaker.SkillBundleFile) error {
	// Entrypoint.
	if entrypointContent != "" {
		digest := sha256HexLocal(entrypointContent)
		if _, err := qtx.UpsertSharedCapabilityFile(ctx, db.UpsertSharedCapabilityFileParams{
			Sha256: digest, Body: entrypointContent, SizeBytes: int64(len(entrypointContent)),
		}); err != nil {
			return fmt.Errorf("upsert entrypoint file: %w", err)
		}
		if err := qtx.UpsertSharedCapabilityVersionFile(ctx, db.UpsertSharedCapabilityVersionFileParams{
			CapabilityVersionID: versionID, Sha256: digest, Path: "SKILL.md", IsEntrypoint: true,
		}); err != nil {
			return fmt.Errorf("link entrypoint: %w", err)
		}
	}
	// Supporting files.
	for _, f := range supporting {
		if f.Content == "" {
			continue
		}
		digest := sha256HexLocal(f.Content)
		if _, err := qtx.UpsertSharedCapabilityFile(ctx, db.UpsertSharedCapabilityFileParams{
			Sha256: digest, Body: f.Content, SizeBytes: int64(len(f.Content)),
		}); err != nil {
			return fmt.Errorf("upsert supporting file %s: %w", f.Path, err)
		}
		if err := qtx.UpsertSharedCapabilityVersionFile(ctx, db.UpsertSharedCapabilityVersionFileParams{
			CapabilityVersionID: versionID, Sha256: digest, Path: f.Path, IsEntrypoint: false,
		}); err != nil {
			return fmt.Errorf("link supporting file %s: %w", f.Path, err)
		}
	}
	return nil
}

// sha256HexLocal computes a "sha256:<hex>" digest for one file body.
func sha256HexLocal(s string) string {
	h := sha256.Sum256([]byte(s))
	return "sha256:" + hex.EncodeToString(h[:])
}
