package handler

import (
	"encoding/json"
	"testing"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestAgentSourceSnapshotListToResponseOnlyIncludesLatestPreviewManifest(t *testing.T) {
	snaps := []db.AgentSourceSnapshot{
		{Status: "preview", Manifest: json.RawMessage(`{"roles":[{"id":"latest"}]}`)},
		{Status: "preview", Manifest: json.RawMessage(`{"roles":[{"id":"older"}]}`)},
		{Status: "applied", Manifest: json.RawMessage(`{"roles":[{"id":"applied"}]}`)},
	}

	got := agentSourceSnapshotListToResponse(snaps)
	if len(got) != 3 {
		t.Fatalf("len(response) = %d, want 3", len(got))
	}
	if string(got[0].Manifest) != string(snaps[0].Manifest) {
		t.Fatalf("latest preview manifest = %s, want %s", got[0].Manifest, snaps[0].Manifest)
	}
	for i := 1; i < len(got); i++ {
		if string(got[i].Manifest) != "null" {
			t.Errorf("response[%d] manifest = %s, want null", i, got[i].Manifest)
		}
	}
}

func TestAgentSourceSnapshotListToResponseOmitsAllManifestsWithoutPreview(t *testing.T) {
	snaps := []db.AgentSourceSnapshot{
		{Status: "applied", Manifest: json.RawMessage(`{"roles":[]}`)},
		{Status: "superseded", Manifest: json.RawMessage(`{"roles":[]}`)},
	}

	got := agentSourceSnapshotListToResponse(snaps)
	for i := range got {
		if string(got[i].Manifest) != "null" {
			t.Errorf("response[%d] manifest = %s, want null", i, got[i].Manifest)
		}
	}
}

func TestAgentSourceSnapshotToResponseNormalizesNullDiagnostics(t *testing.T) {
	got := agentSourceSnapshotToResponse(db.AgentSourceSnapshot{
		Diagnostics: json.RawMessage("null"),
	})

	if got.Diagnostics == nil {
		t.Fatal("diagnostics = nil, want empty array")
	}
	if len(got.Diagnostics) != 0 {
		t.Fatalf("len(diagnostics) = %d, want 0", len(got.Diagnostics))
	}
}
