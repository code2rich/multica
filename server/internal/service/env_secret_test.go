package service

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/util/secretbox"
)

// A 32-byte test key (base64 of 32 zero-edged bytes would also work; we use a
// distinct test key so the test is self-contained).
func testKey(t *testing.T) []byte {
	t.Helper()
	raw := make([]byte, secretbox.KeySize)
	for i := range raw {
		raw[i] = byte(i) // deterministic, 32 distinct bytes
	}
	return raw
}

func TestEnvSecretService_SealOpenRoundTrip(t *testing.T) {
	box, err := secretbox.New(testKey(t))
	if err != nil {
		t.Fatalf("secretbox.New: %v", err)
	}
	svc, err := NewEnvSecretService(box)
	if err != nil {
		t.Fatalf("NewEnvSecretService: %v", err)
	}
	values := map[string]string{
		"AGENT_WORK_DIR":   "/tmp/workdir",
		"PLATFORM_API_KEY": "super-secret-value-do-not-leak",
	}
	sealed, err := svc.SealEnv(values)
	if err != nil {
		t.Fatalf("SealEnv: %v", err)
	}
	if sealed == nil {
		t.Fatal("sealed blob must be non-nil for non-empty values")
	}
	opened, err := svc.OpenEnv(sealed)
	if err != nil {
		t.Fatalf("OpenEnv: %v", err)
	}
	for k, v := range values {
		if opened[k] != v {
			t.Errorf("round-trip mismatch for %s: got %q want %q", k, opened[k], v)
		}
	}
}

// TestEnvSecretService_SealedBlobHasNoPlaintext is the core at-rest guarantee:
// the ciphertext stored in agent.custom_env_encrypted must not contain the
// plaintext value. This proves the database column is value-free at rest.
func TestEnvSecretService_SealedBlobHasNoPlaintext(t *testing.T) {
	box, err := secretbox.New(testKey(t))
	if err != nil {
		t.Fatalf("secretbox.New: %v", err)
	}
	svc, err := NewEnvSecretService(box)
	if err != nil {
		t.Fatalf("NewEnvSecretService: %v", err)
	}
	canary := "super-secret-value-do-not-leak"
	values := map[string]string{
		"PLATFORM_API_KEY": canary,
	}
	sealed, err := svc.SealEnv(values)
	if err != nil {
		t.Fatalf("SealEnv: %v", err)
	}
	if bytes.Contains(sealed, []byte(canary)) {
		t.Fatalf("PLAINTEXT LEAKED into sealed blob: %x", sealed)
	}
	// Also confirm a base64 rendering (how a DB column might be dumped) is clean.
	if strings.Contains(base64.StdEncoding.EncodeToString(sealed), canary) {
		t.Fatal("PLAINTEXT LEAKED through base64 rendering of sealed blob")
	}
}

func TestEnvSecretService_NilBoxRejected(t *testing.T) {
	if _, err := NewEnvSecretService(nil); err == nil {
		t.Fatal("NewEnvSecretService must reject a nil box")
	}
}

func TestEnvSecretService_EmptyValuesSealToNil(t *testing.T) {
	box, _ := secretbox.New(testKey(t))
	svc, _ := NewEnvSecretService(box)
	sealed, err := svc.SealEnv(map[string]string{})
	if err != nil {
		t.Fatalf("SealEnv empty: %v", err)
	}
	if sealed != nil {
		t.Errorf("empty values should seal to nil, got %d bytes", len(sealed))
	}
	// And OpenEnv(nil) returns an empty map (not an error).
	opened, err := svc.OpenEnv(nil)
	if err != nil {
		t.Fatalf("OpenEnv(nil): %v", err)
	}
	if len(opened) != 0 {
		t.Errorf("OpenEnv(nil) = %v, want empty", opened)
	}
}

// TestEnvSecretService_DistinctNoncePerSeal confirms two seals of the same
// values produce different ciphertexts (random nonce), so observers cannot
// correlate identical plaintexts across agents by ciphertext equality.
func TestEnvSecretService_DistinctNoncePerSeal(t *testing.T) {
	box, _ := secretbox.New(testKey(t))
	svc, _ := NewEnvSecretService(box)
	values := map[string]string{"K": "v"}
	a, _ := svc.SealEnv(values)
	b, _ := svc.SealEnv(values)
	if bytes.Equal(a, b) {
		t.Fatal("two seals of the same values must differ (random nonce)")
	}
}
