package agentwaker

import (
	"encoding/json"
	"strings"
	"testing"
)

// secretCanary is the plaintext value placed in the fixture's .env. Every test
// that touches sanitized output must assert this string never appears.
const secretCanary = "super-secret-value-do-not-leak"

func TestSanitizeEnvForPreview_StripsPlaintext(t *testing.T) {
	env, err := ParseEnvFile([]byte("PLATFORM_API_KEY=" + secretCanary + "\nAGENT_WORK_DIR=/path\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	key := []byte("32-byte-server-secret-key-for-hmac!!")
	safe := SanitizeEnvForPreview(env, key)

	// Serialize the entire sanitized output and assert the canary is absent.
	b, err := json.Marshal(safe)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), secretCanary) {
		t.Fatalf("PLAINTEXT LEAKED into sanitized output: %s", b)
	}
	// No declaration carries a "value" field; only "value_digest".
	for _, d := range safe {
		blob, _ := json.Marshal(d)
		if strings.Contains(string(blob), "\"value\":") {
			t.Fatalf("sanitized declaration carried a plaintext value field: %s", blob)
		}
	}
}

func TestSanitizeEnvForPreview_DigestPresentOnlyWhenConfiguredAndKeyed(t *testing.T) {
	env, _ := ParseEnvFile([]byte("FOO=bar\n"))
	decl := env.Declarations["FOO"]
	decl.Required = true
	env.Declarations["FOO"] = decl

	t.Run("with key → digest present", func(t *testing.T) {
		safe := SanitizeEnvForPreview(env, []byte("32-byte-server-secret-key-for-hmac!!"))
		if safe[0].ValueDigest == "" {
			t.Error("configured + keyed value must produce a digest")
		}
		if !safe[0].Configured {
			t.Error("FOO should be configured")
		}
	})
	t.Run("without key → no digest, still no value", func(t *testing.T) {
		safe := SanitizeEnvForPreview(env, nil)
		if safe[0].ValueDigest != "" {
			t.Error("no digest when hmac key absent")
		}
		blob, _ := json.Marshal(safe[0])
		if strings.Contains(string(blob), secretCanary) || strings.Contains(string(blob), "bar") {
			t.Errorf("plaintext leaked even without key: %s", blob)
		}
	})
}

func TestSanitizeEnvForPreview_PreservesOrder(t *testing.T) {
	content := "Z=1\nA=2\nM=3\n"
	env, _ := ParseEnvFile([]byte(content))
	safe := SanitizeEnvForPreview(env, nil)
	want := []string{"Z", "A", "M"}
	for i, w := range want {
		if safe[i].Name != w {
			t.Errorf("safe[%d].Name = %q, want %q", i, safe[i].Name, w)
		}
	}
}

func TestAssertNoPlaintextEnv(t *testing.T) {
	t.Run("clean sanitized output passes", func(t *testing.T) {
		safe := map[string]any{
			"env": []any{
				map[string]any{"name": "KEY", "configured": true, "value_digest": "hmac-sha256:abc"},
			},
		}
		if hit := AssertNoPlaintextEnv(safe); hit != "" {
			t.Errorf("clean output flagged at %q", hit)
		}
	})
	t.Run("bare value field rejected", func(t *testing.T) {
		leaking := map[string]any{
			"env": []any{
				map[string]any{"name": "KEY", "value": secretCanary},
			},
		}
		hit := AssertNoPlaintextEnv(leaking)
		if hit == "" {
			t.Fatal("leaking output was not detected")
		}
		if !strings.Contains(hit, "value") {
			t.Errorf("detector pointed at %q, expected a 'value' path", hit)
		}
	})
	t.Run("nested value rejected", func(t *testing.T) {
		leaking := map[string]any{
			"role": map[string]any{
				"env": map[string]any{
					"items": []any{
						map[string]any{"name": "K", "value": "x"},
					},
				},
			},
		}
		if hit := AssertNoPlaintextEnv(leaking); hit == "" {
			t.Fatal("nested leak not detected")
		}
	})
	t.Run("array index path", func(t *testing.T) {
		leaking := map[string]any{
			"items": []any{
				map[string]any{"ok": true},
				map[string]any{"value": "leak"},
			},
		}
		hit := AssertNoPlaintextEnv(leaking)
		if hit == "" || !strings.Contains(hit, "[1]") {
			t.Fatalf("array leak not localized: %q", hit)
		}
	})
}
