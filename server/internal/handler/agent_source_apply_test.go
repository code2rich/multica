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
		"mission":                 "Create durable agents",
		"description_zh":          "从真实意图中唤醒可信的数字同事",
		"instructions_content":    "# Full instructions",
		"instructions_content_zh": "# 中文展示指令",
		"persona_content":         "<section>Persona</section>",
		"source_files": []any{
			map[string]any{"path": "agent-soul/PROFILE.yaml", "content": "id: creator"},
		},
		"mcp": map[string]any{"has_servers": false},
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
	if params.InstructionsZh != "# 中文展示指令" {
		t.Fatalf("Chinese display instructions not populated: %q", params.InstructionsZh)
	}
	if params.Description != "从真实意图中唤醒可信的数字同事" {
		t.Fatalf("Chinese display description not used: %q", params.Description)
	}
	if !params.ProfileHtml.Valid || params.ProfileHtml.String != "<section>Persona</section>" {
		t.Fatalf("persona HTML not populated: %+v", params.ProfileHtml)
	}
	if string(params.SourceFiles) != `[{"content":"id: creator","path":"agent-soul/PROFILE.yaml"}]` {
		t.Fatalf("source files not populated: %s", params.SourceFiles)
	}
}

func TestDescriptionFallsBackToMissionAndTruncatesUnicodeSafely(t *testing.T) {
	if got := descriptionOf(map[string]any{"mission": "English fallback"}); got != "English fallback" {
		t.Fatalf("mission fallback mismatch: %q", got)
	}
	if got := truncate("中文描述", 2); got != "中文" {
		t.Fatalf("Unicode truncation mismatch: %q", got)
	}
}

func TestSourceManagedSkillCreateInputUsesApplyingUser(t *testing.T) {
	workspaceID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	ownerID := pgtype.UUID{Bytes: [16]byte{3}, Valid: true}
	input := sourceManagedSkillCreateInput(ApplySnapshotInput{
		WorkspaceID: workspaceID,
		OwnerID:     ownerID,
		SourceID:    pgtype.UUID{Bytes: [16]byte{4}, Valid: true},
	}, "creator", "creator-skill", "Creator Skill", "Create agents", "创建角色", "sha256:test")

	if input.WorkspaceID != workspaceID || input.CreatorID != ownerID {
		t.Fatalf("source skill must retain workspace and applying user: %+v", input)
	}
	if input.Description != "Create agents" || input.DescriptionZH != "创建角色" {
		t.Fatalf("localized skill descriptions not preserved: %+v", input)
	}
}

func TestEnvValuesFromRoleSourceFilesParsesExactEnvBody(t *testing.T) {
	role := map[string]any{
		"source_files": []any{
			map[string]any{"path": "secrets.env", "content": "OUT_OF_SCOPE=wrong"},
			map[string]any{"path": "env/.env", "content": "TOKEN='secret value'\nEMPTY=\n"},
		},
	}
	got, err := envValuesFromRoleSourceFiles(role)
	if err != nil {
		t.Fatalf("envValuesFromRoleSourceFiles: %v", err)
	}
	if got["TOKEN"] != "secret value" || got["EMPTY"] != "" {
		t.Fatalf("unexpected parsed values: %#v", got)
	}
	if _, loaded := got["OUT_OF_SCOPE"]; loaded {
		t.Fatal("parsed a non-canonical dotenv source file")
	}
}

func TestEnvValuesFromRoleSourceFilesReturnsEmptyWhenAbsent(t *testing.T) {
	got, err := envValuesFromRoleSourceFiles(map[string]any{})
	if err != nil {
		t.Fatalf("envValuesFromRoleSourceFiles: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty env values, got %#v", got)
	}
}
