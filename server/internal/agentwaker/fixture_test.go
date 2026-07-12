package agentwaker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureDir is the minimal AgentWaker tree checked in under testdata.
func fixtureDir(t *testing.T) string {
	t.Helper()
	// Tests run from the package directory (server/internal/agentwaker);
	// testdata lives at server/testdata, i.e. two levels up.
	dir := filepath.Join("..", "..", "testdata", "agentwaker-fixture")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("fixture missing at %s: %v", dir, err)
	}
	return dir
}

func readFile(t *testing.T, path ...string) []byte {
	t.Helper()
	full := filepath.Join(append([]string{fixtureDir(t)}, path...)...)
	b, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read %s: %v", full, err)
	}
	return b
}

// TestFixture_RegistryAndCapabilities parses the real fixture contracts and
// confirms every capability is well-formed.
func TestFixture_RegistryAndCapabilities(t *testing.T) {
	registry, err := ParseRegistry(readFile(t, "capabilities", "registry.yaml"))
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	if len(registry.Capabilities) != 2 {
		t.Fatalf("want 2 capabilities in fixture registry, got %d", len(registry.Capabilities))
	}
	for _, entry := range registry.Capabilities {
		t.Run(entry.ID, func(t *testing.T) {
			manifest, err := ParseCapabilityManifest(readFile(t, "capabilities", entry.ID, "CAPABILITY.yaml"))
			if err != nil {
				t.Fatalf("capability %s manifest: %v", entry.ID, err)
			}
			if manifest.Version != entry.Version {
				t.Errorf("registry/manifest version mismatch: %q vs %q", entry.Version, manifest.Version)
			}
			if len(manifest.Profiles) == 0 {
				t.Errorf("capability %s must declare at least one profile", entry.ID)
			}
		})
	}
}

// TestFixture_Roles parses both roles' PROFILE.yaml + capabilities.yaml and
// confirms the dependency manifest matches the profiles available.
func TestFixture_Roles(t *testing.T) {
	// Build a quick capability → profiles index from the registry.
	registry, _ := ParseRegistry(readFile(t, "capabilities", "registry.yaml"))
	profilesByID := make(map[string]map[string]bool)
	for _, entry := range registry.Capabilities {
		manifest, err := ParseCapabilityManifest(readFile(t, "capabilities", entry.ID, "CAPABILITY.yaml"))
		if err != nil {
			t.Fatalf("capability %s: %v", entry.ID, err)
		}
		set := make(map[string]bool)
		for _, p := range manifest.Profiles {
			set[p.ID] = true
		}
		profilesByID[entry.ID] = set
	}

	for _, role := range []string{"research-operator", "plain-operator"} {
		t.Run(role, func(t *testing.T) {
			profile, err := ParseProfile(readFile(t, role, "agent-soul", "PROFILE.yaml"))
			if err != nil {
				t.Fatalf("profile: %v", err)
			}
			if profile.ID != role {
				t.Errorf("profile id %q != dir %q", profile.ID, role)
			}
			rc, err := ParseRoleCapabilities(readFile(t, role, "capabilities.yaml"))
			if err != nil {
				t.Fatalf("capabilities.yaml: %v", err)
			}
			// Every used_by profile must exist on the referenced capability.
			for _, b := range rc.Capabilities {
				available, ok := profilesByID[b.ID]
				if !ok {
					t.Errorf("role %s binds unknown capability %q", role, b.ID)
					continue
				}
				for _, u := range b.UsedBy {
					if !available[u.Profile] {
						t.Errorf("role %s capability %s profile %q not declared", role, b.ID, u.Profile)
					}
				}
			}
		})
	}
}

// TestFixture_EnvRedaction is the canonical proof that plaintext env values
// never leave the sanitization boundary. It reads the real fixture .env
// (containing secretCanary), sanitizes, and asserts the canary is absent from
// every serialization of the sanitized output.
func TestFixture_EnvRedaction(t *testing.T) {
	example, err := ParseEnvFile(readFile(t, "research-operator", "env", ".env.example"))
	if err != nil {
		t.Fatalf("parse .env.example: %v", err)
	}
	real, err := ParseEnvFile(readFile(t, "research-operator", "env", ".env"))
	if err != nil {
		t.Fatalf("parse .env: %v", err)
	}
	// Sanity: the real fixture really does contain the canary.
	if !strings.Contains(real.Values["PLATFORM_API_KEY"], secretCanary) {
		t.Fatalf("fixture .env does not contain expected canary value; test is ineffective")
	}

	merged := MergeEnvDeclarations(example, real)
	safe := SanitizeEnvForPreview(merged, []byte("32-byte-server-secret-key-for-hmac!!"))

	// 1. Direct struct serialization.
	if blob, _ := json.Marshal(safe); strings.Contains(string(blob), secretCanary) {
		t.Fatalf("PLAINTEXT LEAKED via struct marshal: %s", blob)
	}
	// 2. Each declaration individually.
	for _, d := range safe {
		if blob, _ := json.Marshal(d); strings.Contains(string(blob), secretCanary) {
			t.Fatalf("PLAINTEXT LEAKED in declaration %q: %s", d.Name, blob)
		}
	}
	// 3. Defense-in-depth detector agrees the sanitized form is clean.
	if hit := AssertNoPlaintextEnv(map[string]any{"env": toAnySlice(safe)}); hit != "" {
		t.Fatalf("sanitized output flagged by detector at %q", hit)
	}
	// 4. Configured booleans and digests are present for configured keys.
	for _, name := range []string{"PLATFORM_API_KEY", "PLATFORM_API_BASE", "AGENT_WORK_DIR"} {
		found := false
		for _, d := range safe {
			if d.Name == name {
				found = true
				if !d.Configured {
					t.Errorf("%s should be configured", name)
				}
				if d.ValueDigest == "" {
					t.Errorf("%s should have a digest", name)
				}
			}
		}
		if !found {
			t.Errorf("configured key %s missing from sanitized output", name)
		}
	}
}

// TestFixture_PlaintextDetectorCatchesLeak proves the detector WOULD catch the
// leak if sanitization were skipped — establishing the test has teeth.
func TestFixture_PlaintextDetectorCatchesLeak(t *testing.T) {
	real, _ := ParseEnvFile(readFile(t, "research-operator", "env", ".env"))
	// Deliberately construct the leaking shape (value as plaintext).
	leaking := map[string]any{
		"env": []any{
			map[string]any{"name": "PLATFORM_API_KEY", "value": real.Values["PLATFORM_API_KEY"]},
		},
	}
	hit := AssertNoPlaintextEnv(leaking)
	if hit == "" {
		t.Fatal("detector failed to catch a deliberate plaintext leak — test is ineffective")
	}
}

// TestFixture_BinaryAssetPresent confirms the binary skip-path fixture exists
// so M1 scanner tests can assert it is excluded from text bundles.
func TestFixture_BinaryAssetPresent(t *testing.T) {
	p := filepath.Join(fixtureDir(t), "research-operator", "research-operator-skills", "trend-research", "cover.png")
	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("binary fixture missing: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("binary fixture is empty")
	}
}

// TestFixture_UnrelatedEnvFileIsNotTheRoleEnv confirms that the unrelated
// secrets.env file is a SEPARATE file from the recognized env/.env, so the M1
// scanner's exact-path scoping rule (only {role}/env/.env) is testable.
func TestFixture_UnrelatedEnvFileIsNotTheRoleEnv(t *testing.T) {
	canary, err := os.ReadFile(filepath.Join(fixtureDir(t), "research-operator", "secrets.env"))
	if err != nil {
		t.Fatalf("unrelated env fixture missing: %v", err)
	}
	if !strings.Contains(string(canary), "unrelated-secret-must-not-be-read") {
		t.Fatal("unrelated env fixture lost its canary; scoping test is ineffective")
	}
	// And confirm the recognized path is different and also present.
	if _, err := os.Stat(filepath.Join(fixtureDir(t), "research-operator", "env", ".env")); err != nil {
		t.Fatal("recognized env/.env missing")
	}
}

// toAnySlice converts typed declarations to []any for the detector.
func toAnySlice(safe []SanitizedEnvDeclaration) []any {
	out := make([]any, len(safe))
	for i, d := range safe {
		out[i] = d
	}
	return out
}
