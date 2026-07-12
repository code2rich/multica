package handler

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestSourceManagedAgentCreateParamsPopulateRequiredJSONFields(t *testing.T) {
	workspaceID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	runtimeID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	ownerID := pgtype.UUID{Bytes: [16]byte{3}, Valid: true}
	params := sourceManagedAgentCreateParams(workspaceID, runtimeID, ownerID, "Carter", map[string]any{
		"mission":              "Create durable agents",
		"instructions_content": "# Full instructions",
		"persona_content":      "<section>Persona</section>",
		"mcp":                  map[string]any{"has_servers": false},
	})

	if string(params.RuntimeConfig) != "{}" {
		t.Fatalf("runtime_config must be a non-NULL JSON object, got %q", params.RuntimeConfig)
	}
	if string(params.CustomEnv) != "{}" {
		t.Fatalf("custom_env must be a non-NULL JSON object, got %q", params.CustomEnv)
	}
	if string(params.CustomArgs) != "[]" {
		t.Fatalf("custom_args must be a non-NULL JSON array, got %q", params.CustomArgs)
	}
	if len(params.McpConfig) == 0 {
		t.Fatal("mcp_config must be explicit JSON, not SQL NULL")
	}
	if params.MaxConcurrentTasks != 6 {
		t.Fatalf("max_concurrent_tasks must use the product default 6, got %d", params.MaxConcurrentTasks)
	}
	if params.RuntimeID != runtimeID || params.WorkspaceID != workspaceID {
		t.Fatalf("workspace/runtime identity not preserved: %+v", params)
	}
	if params.OwnerID != ownerID {
		t.Fatalf("applying user must own an imported agent: got %+v want %+v", params.OwnerID, ownerID)
	}
	if params.Instructions != "# Full instructions" {
		t.Fatalf("full agent-detail content not used: %q", params.Instructions)
	}
	if !params.ProfileHtml.Valid || params.ProfileHtml.String != "<section>Persona</section>" {
		t.Fatalf("persona HTML not populated: %+v", params.ProfileHtml)
	}
}

func TestSourceManagedSkillCreateInputUsesApplyingUser(t *testing.T) {
	workspaceID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	ownerID := pgtype.UUID{Bytes: [16]byte{3}, Valid: true}
	input := sourceManagedSkillCreateInput(ApplySnapshotInput{
		WorkspaceID: workspaceID,
		OwnerID:     ownerID,
		SourceID:    pgtype.UUID{Bytes: [16]byte{4}, Valid: true},
	}, "creator", "creator-skill", "Creator Skill", "sha256:test")

	if input.WorkspaceID != workspaceID || input.CreatorID != ownerID {
		t.Fatalf("source skill must retain workspace and applying user: %+v", input)
	}
}
