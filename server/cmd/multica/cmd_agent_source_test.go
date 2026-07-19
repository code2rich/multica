package main

import "testing"

func TestAgentSourceScanSummaryOmitsManifest(t *testing.T) {
	input := map[string]any{
		"id":             "scan-1",
		"status":         "completed",
		"directory_hash": "sha256:abc",
		"manifest": map[string]any{
			"source_files": []any{
				map[string]any{"path": "env/.env", "content": "TOKEN=secret"},
			},
		},
		"diagnostics": []any{},
	}

	got := agentSourceScanSummary(input)
	if _, ok := got["manifest"]; ok {
		t.Fatal("scan summary must omit manifest")
	}
	if got["id"] != "scan-1" || got["status"] != "completed" {
		t.Fatalf("scan summary lost safe metadata: %#v", got)
	}
	if _, ok := input["manifest"]; !ok {
		t.Fatal("scan summary must not mutate the original response")
	}
}
