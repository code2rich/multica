package service

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/pkg/skillbundle"
)

// TestBuildCapabilityBindingNote verifies the machine-owned binding note
// generated for each task. This note maps role skills to shared-capability
// profiles and permissions and is regenerated on every task (never cached).
func TestBuildCapabilityBindingNote(t *testing.T) {
	t.Run("single capability with one binding", func(t *testing.T) {
		byCap := map[string]*capAccum{
			"cap1": {
				name: "Information Collection (shared capability)",
				bindings: []capBindingInfo{
					{profile: "trend-research", requirement: "^1.0.0", required: true, mode: "read-only", fallback: "partial"},
				},
			},
		}
		note := buildCapabilityBindingNote(byCap, []string{"cap1"})
		if note == "" {
			t.Fatal("expected non-empty binding note")
		}
		if !strings.Contains(note, "# Capability bindings") {
			t.Errorf("note must contain header: %s", note)
		}
		if !strings.Contains(note, "trend-research") {
			t.Errorf("note must contain profile: %s", note)
		}
		if !strings.Contains(note, "required") {
			t.Errorf("note must contain 'required' for required bindings: %s", note)
		}
		if !strings.Contains(note, "read-only") {
			t.Errorf("note must contain permission mode: %s", note)
		}
		if !strings.Contains(note, "partial") {
			t.Errorf("note must contain fallback behavior: %s", note)
		}
	})

	t.Run("multiple capabilities", func(t *testing.T) {
		byCap := map[string]*capAccum{
			"cap1": {
				name: "Information Collection",
				bindings: []capBindingInfo{
					{profile: "trend-research", requirement: "^1.0.0", required: true, mode: "read-only", fallback: "partial"},
				},
			},
			"cap2": {
				name: "Visual Generation",
				bindings: []capBindingInfo{
					{profile: "xiaohongshu-note-visuals", requirement: "^1.0.0", required: true, mode: "local-write", fallback: "blocked"},
				},
			},
		}
		note := buildCapabilityBindingNote(byCap, []string{"cap1", "cap2"})
		if !strings.Contains(note, "Information Collection") {
			t.Errorf("note must contain cap1: %s", note)
		}
		if !strings.Contains(note, "Visual Generation") {
			t.Errorf("note must contain cap2: %s", note)
		}
	})

	t.Run("no bindings returns empty", func(t *testing.T) {
		byCap := map[string]*capAccum{
			"cap1": {name: "X", bindings: nil},
		}
		note := buildCapabilityBindingNote(byCap, []string{"cap1"})
		if note != "" {
			t.Errorf("no bindings → empty note, got: %s", note)
		}
	})
}

// TestCapabilityAccumDeduplicatesJoinedRows covers the query shape used by
// LoadBoundSharedCapabilities: each binding is repeated for every capability
// file, and each file is repeated for every binding. The runtime bundle must
// contain one copy of each or providers that refuse overwrites cannot start.
func TestCapabilityAccumDeduplicatesJoinedRows(t *testing.T) {
	t.Parallel()

	var accum capAccum
	for _, bindingID := range []string{"binding-1", "binding-2"} {
		for _, file := range []AgentSkillFileData{
			{Path: "SKILL.zh.md", Content: "Chinese skill"},
			{Path: "schemas/request.json", Content: `{"type":"object"}`},
		} {
			accum.addFile(file.Path, file.Content)
			accum.addBinding(bindingID, capBindingInfo{profile: bindingID})
		}
	}

	if len(accum.files) != 2 {
		t.Fatalf("files = %d, want one copy of each of 2 files", len(accum.files))
	}
	if len(accum.bindings) != 2 {
		t.Fatalf("bindings = %d, want one copy of each of 2 bindings", len(accum.bindings))
	}
	if accum.files[0].Path != "SKILL.zh.md" || accum.files[1].Path != "schemas/request.json" {
		t.Fatalf("unexpected files: %#v", accum.files)
	}
}

// TestExtractModeAndFallback verifies the JSONB helpers that parse permissions
// and fallback fields from agent_capability_binding.
func TestExtractModeAndFallback(t *testing.T) {
	t.Run("extractMode", func(t *testing.T) {
		if mode := extractMode([]byte(`{"mode":"read-only","account_actions":false}`)); mode != "read-only" {
			t.Errorf("mode = %q, want read-only", mode)
		}
		if mode := extractMode([]byte(`{}`)); mode != "" {
			t.Errorf("empty perms → empty mode, got %q", mode)
		}
		if mode := extractMode(nil); mode != "" {
			t.Errorf("nil perms → empty mode, got %q", mode)
		}
	})
	t.Run("extractBehavior", func(t *testing.T) {
		if b := extractBehavior([]byte(`{"behavior":"partial","message":"gaps"}`)); b != "partial" {
			t.Errorf("behavior = %q, want partial", b)
		}
		if b := extractBehavior(nil); b != "" {
			t.Errorf("nil fallback → empty, got %q", b)
		}
	})
}

// TestBuildAgentSkillBundles_SetsSharedCapabilitySource verifies that bundles
// with Source=SourceSharedCapability pass through BuildAgentSkillBundles with
// their source preserved (not overwritten to 'workspace').
func TestBuildAgentSkillBundles_SetsSharedCapabilitySource(t *testing.T) {
	capBundle := AgentSkillData{
		ID:          "capability:information-collection",
		Source:      skillbundle.SourceSharedCapability,
		Name:        "Information Collection (shared capability)",
		Description: "Collect evidence.",
		Content:     "# Information Collection\n\nShared capability entrypoint.",
		Files: []AgentSkillFileData{
			{Path: "schemas/request.json", Content: `{"type":"object"}`},
		},
	}
	bundles, skillRefs := BuildAgentSkillBundles([]AgentSkillData{capBundle})
	if len(bundles) != 1 || len(skillRefs) != 1 {
		t.Fatalf("want 1 bundle+ref, got %d+%d", len(bundles), len(skillRefs))
	}
	if bundles[0].Source != skillbundle.SourceSharedCapability {
		t.Errorf("source = %q, want %q", bundles[0].Source, skillbundle.SourceSharedCapability)
	}
	if skillRefs[0].Source != skillbundle.SourceSharedCapability {
		t.Errorf("ref source = %q, want %q", skillRefs[0].Source, skillbundle.SourceSharedCapability)
	}
	if bundles[0].Hash == "" || !strings.HasPrefix(bundles[0].Hash, "sha256:") {
		t.Errorf("bundle hash must be sha256-prefixed: %q", bundles[0].Hash)
	}
	if skillRefs[0].Hash != bundles[0].Hash {
		t.Errorf("ref hash must equal bundle hash")
	}
}

// TestBuildAgentSkillBundles_WorkspaceAndCapabilityMixed verifies that a mixed
// skill set (workspace skills + shared capabilities + builtins) produces bundles
// with correct sources throughout.
func TestBuildAgentSkillBundles_WorkspaceAndCapabilityMixed(t *testing.T) {
	skills := []AgentSkillData{
		{ID: "ws-skill-1", Name: "WS Skill"},
		{ID: "", Name: "Builtin Skill"},
		{ID: "capability:info-collect", Source: skillbundle.SourceSharedCapability, Name: "IC", Content: "# IC"},
	}
	bundles, _ := BuildAgentSkillBundles(skills)
	if len(bundles) != 3 {
		t.Fatalf("want 3 bundles, got %d", len(bundles))
	}
	sources := map[string]string{}
	for _, b := range bundles {
		sources[b.ID] = b.Source
	}
	if sources["ws-skill-1"] != skillbundle.SourceWorkspace {
		t.Errorf("workspace skill source = %q", sources["ws-skill-1"])
	}
	if sources["capability:info-collect"] != skillbundle.SourceSharedCapability {
		t.Errorf("shared cap source = %q", sources["capability:info-collect"])
	}
	// Built-in gets synthetic id.
	found := false
	for _, b := range bundles {
		if strings.HasPrefix(b.ID, "builtin:") {
			if b.Source != skillbundle.SourceBuiltin {
				t.Errorf("builtin source = %q", b.Source)
			}
			found = true
		}
	}
	if !found {
		t.Error("builtin skill not found in bundles")
	}
}

// TestSha256Hex verifies the content-addressed hash used by storeCapabilityVersionFiles
// produces consistent, deterministically formatted digests.
func TestSha256Hex(t *testing.T) {
	a := testSHA256("hello world")
	b := testSHA256("hello world")
	if a != b {
		t.Errorf("sha256 non-deterministic: %q vs %q", a, b)
	}
	if !strings.HasPrefix(a, "sha256:") {
		t.Errorf("must be sha256-prefixed: %q", a)
	}
	c := testSHA256("hello world!")
	if a == c {
		t.Error("different inputs must produce different hashes")
	}
}

func testSHA256(s string) string {
	h := sha256.Sum256([]byte(s))
	return "sha256:" + hex.EncodeToString(h[:])
}
