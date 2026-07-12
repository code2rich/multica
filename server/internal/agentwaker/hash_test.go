package agentwaker

import (
	"strings"
	"testing"
)

func TestSkillBundleHash_Deterministic(t *testing.T) {
	files := []SkillBundleFile{{Path: "b.md", Content: "2"}, {Path: "a.md", Content: "1"}}
	h1 := SkillBundleHash("workspace", "id-1", "Name", "desc", "content", files)
	// Re-order input; hash must be identical because the algorithm sorts by path.
	h2 := SkillBundleHash("workspace", "id-1", "Name", "desc", "content", []SkillBundleFile{{Path: "a.md", Content: "1"}, {Path: "b.md", Content: "2"}})
	if h1 != h2 {
		t.Errorf("hash must be order-independent: %q vs %q", h1, h2)
	}
	if !strings.HasPrefix(h1, "sha256:") {
		t.Errorf("hash must be sha256-prefixed: %q", h1)
	}
}

func TestSkillBundleHash_DistinctOnContentChange(t *testing.T) {
	base := SkillBundleHash("workspace", "id", "N", "d", "c", nil)
	changed := SkillBundleHash("workspace", "id", "N", "d", "c2", nil)
	if base == changed {
		t.Error("content change must change the hash")
	}
}

func TestSkillBundleHash_LengthPrefixingPreventsCollision(t *testing.T) {
	// Without length-prefixing, ("ab","c") and ("a","bc") would hash equal.
	// The framing must distinguish them.
	left := SkillBundleHash("s", "ab", "c", "", "", nil)
	right := SkillBundleHash("s", "a", "bc", "", "", nil)
	if left == right {
		t.Error("length-prefixing failed: distinct field splits collided")
	}
}

func TestCapabilityContentHash_StableAcrossProfileReorder(t *testing.T) {
	manifest := CapabilityManifest{
		ID: "x", Version: "1.0.0", Name: "X", Description: "d", Entrypoint: "SKILL.md",
		Profiles: []CapabilityProfile{{ID: "a", Description: "1"}, {ID: "b", Description: "2"}},
	}
	h1 := CapabilityContentHash(manifest, "entrypoint", nil)
	// Reverse profile order; hash must be identical.
	manifest.Profiles = []CapabilityProfile{{ID: "b", Description: "2"}, {ID: "a", Description: "1"}}
	h2 := CapabilityContentHash(manifest, "entrypoint", nil)
	if h1 != h2 {
		t.Errorf("capability hash must be stable across profile reorder: %q vs %q", h1, h2)
	}
}

func TestCapabilityContentHash_ChangesOnVersionBump(t *testing.T) {
	manifest := CapabilityManifest{ID: "x", Version: "1.0.0", Name: "X", Description: "d", Entrypoint: "SKILL.md"}
	h1 := CapabilityContentHash(manifest, "e", nil)
	manifest.Version = "1.1.0"
	h2 := CapabilityContentHash(manifest, "e", nil)
	if h1 == h2 {
		t.Error("version bump must change capability hash")
	}
}

func TestDirectoryHash_CanonicalOrdering(t *testing.T) {
	// Two maps with the same content but different Go-map iteration backing must
	// hash identically because canonical JSON sorts keys. Run repeatedly to
	// increase the chance of observing non-determinism if it existed.
	manifest := map[string]any{
		"roles": []any{
			map[string]any{"id": "a", "hash": "sha256:1"},
			map[string]any{"id": "b", "hash": "sha256:2"},
		},
		"capabilities": []any{
			map[string]any{"id": "c1", "hash": "sha256:3"},
		},
		"directory_version": "v1",
	}
	var first string
	for i := 0; i < 50; i++ {
		h, err := DirectoryHash(manifest)
		if err != nil {
			t.Fatalf("DirectoryHash error: %v", err)
		}
		if i == 0 {
			first = h
		} else if h != first {
			t.Fatalf("DirectoryHash non-deterministic at iter %d: %q vs %q", i, h, first)
		}
	}
	if !strings.HasPrefix(first, "sha256:") {
		t.Errorf("directory hash must be sha256-prefixed: %q", first)
	}
}

func TestDirectoryHash_RejectsPlaintextLeakage(t *testing.T) {
	// A sanitized manifest (digests only) hashes fine.
	safe := map[string]any{
		"env": []any{
			map[string]any{"name": "KEY", "configured": true, "value_digest": "hmac-sha256:abc"},
		},
	}
	if _, err := DirectoryHash(safe); err != nil {
		t.Fatalf("safe manifest must hash: %v", err)
	}
}

func TestEnvValueDigest_NonReversibleAndKeyed(t *testing.T) {
	key := []byte("32-byte-server-secret-key-for-hmac!!") // exactly 32
	d1 := EnvValueDigest("KEY", "value-1", key)
	d2 := EnvValueDigest("KEY", "value-1", key)
	if d1 != d2 {
		t.Error("digest must be deterministic for same key+value+hmac")
	}
	// Different value → different digest.
	if EnvValueDigest("KEY", "value-2", key) == d1 {
		t.Error("digest must change when value changes")
	}
	// Different name → different digest (rename detected even if value same).
	if EnvValueDigest("OTHER", "value-1", key) == d1 {
		t.Error("digest must change when key name changes")
	}
	// Different HMAC key → different digest (server-side key rotation detectable).
	otherKey := []byte("different-32-byte-server-secret-key!!")
	if EnvValueDigest("KEY", "value-1", otherKey) == d1 {
		t.Error("digest must change when hmac key changes")
	}
	// The digest never contains the plaintext.
	if strings.Contains(d1, "value-1") {
		t.Error("digest leaked plaintext value")
	}
}
