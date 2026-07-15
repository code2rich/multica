package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/agentwaker"
)

// fixtureRoot returns the absolute path to the shared AgentWaker test fixture.
// The scanner requires an absolute path (it reuses the daemon path validators).
func fixtureRoot(t *testing.T) string {
	t.Helper()
	rel := filepath.Join("..", "..", "testdata", "agentwaker-fixture")
	abs, err := filepath.Abs(rel)
	if err != nil {
		t.Fatalf("resolve fixture abs path: %v", err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("fixture missing at %s: %v", abs, err)
	}
	return abs
}

// scanCanary is the plaintext value in the fixture's research-operator/env/.env.
// Every scan test asserts it never appears in the sanitized scan result.
const scanCanary = "super-secret-value-do-not-leak"

// unrelatedCanary is the value in the OUT-of-scope secrets.env that must NEVER
// be read by the scanner.
const unrelatedCanary = "unrelated-secret-must-not-be-read"

// TestScanDirectory_RedactsPlaintextEnv is the canonical M1 proof: a full
// directory scan produces a sanitized manifest, and plaintext env values never
// appear in it — not in the manifest, not in the diagnostics, not in the
// serialized JSON. It also proves the scanner does not read unrelated .env
// files outside the recognized {role}/env/.env path.
func TestScanDirectory_RedactsPlaintextEnv(t *testing.T) {
	key := []byte("32-byte-server-secret-key-for-hmac!!")
	result, err := ScanDirectory(t.Context(), fixtureRoot(t), key)
	if err != nil {
		t.Fatalf("ScanDirectory failed: %v", err)
	}

	// 1. The configured canary must be absent from the entire serialized result.
	blob, _ := json.Marshal(result)
	if strings.Contains(string(blob), scanCanary) {
		t.Fatalf("PLAINTEXT LEAKED into scan result: %s", blob)
	}

	// 2. The unrelated .env canary must also be absent — proves exact-path
	//    scoping (scanner did not read {role}/secrets.env).
	if strings.Contains(string(blob), unrelatedCanary) {
		t.Fatalf("UNRELATED .ENV LEAKED into scan result (scoping failed): %s", blob)
	}

	// 3. Every env declaration in the manifest uses value_digest, never value.
	walkEnvDecls(result.Manifest, func(decl map[string]any, role string) {
		if _, hasValue := decl["value"]; hasValue {
			t.Errorf("role %s env decl carried a plaintext value field: %v", role, decl)
		}
		if digest, ok := decl["value_digest"].(string); ok && digest != "" {
			if strings.Contains(digest, scanCanary) {
				t.Errorf("role %s value_digest leaked plaintext: %s", role, digest)
			}
		}
	})
}

// TestScanDirectory_DiscoversAllContracts confirms the scanner finds both
// capabilities, both roles, the capability bindings, the skill packages, the
// env declarations, and the MCP declaration.
func TestScanDirectory_DiscoversAllContracts(t *testing.T) {
	// Supply the HMAC key so value digests are produced and verifiable.
	result, err := ScanDirectory(t.Context(), fixtureRoot(t), []byte("32-byte-server-secret-key-for-hmac!!"))
	if err != nil {
		t.Fatalf("ScanDirectory failed: %v", err)
	}

	caps := result.Manifest["capabilities"].([]map[string]any)
	if len(caps) != 2 {
		t.Fatalf("want 2 capabilities, got %d", len(caps))
	}
	capIDs := map[string]bool{}
	for _, cm := range caps {
		capIDs[cm["id"].(string)] = true
		if cm["content_hash"] == "" {
			t.Errorf("capability %s missing content_hash", cm["id"])
		}
	}
	if !capIDs["information-collection"] || !capIDs["visual-generation"] {
		t.Fatalf("missing expected capabilities, got %v", capIDs)
	}

	roles := result.Manifest["roles"].([]map[string]any)
	if len(roles) != 2 {
		t.Fatalf("want 2 roles, got %d", len(roles))
	}
	roleByID := map[string]map[string]any{}
	for _, rm := range roles {
		roleByID[rm["id"].(string)] = rm
	}
	if _, ok := roleByID["research-operator"]; !ok {
		t.Fatal("research-operator role not discovered")
	}
	if _, ok := roleByID["plain-operator"]; !ok {
		t.Fatal("plain-operator role not discovered")
	}

	// research-operator declares 2 capability bindings.
	research := roleByID["research-operator"]
	bindings := research["capability_bindings"].([]map[string]any)
	if len(bindings) != 2 {
		t.Fatalf("research-operator want 2 bindings, got %d", len(bindings))
	}
	// plain-operator declares none.
	plain := roleByID["plain-operator"]
	if pb := plain["capability_bindings"].([]map[string]any); len(pb) != 0 {
		t.Fatalf("plain-operator want 0 bindings, got %d", len(pb))
	}

	// research-operator has a meta skill + 1 specialist skill.
	researchSkills := research["skills"].([]map[string]any)
	if len(researchSkills) != 2 {
		t.Fatalf("research-operator want 2 skills (meta + specialist), got %d", len(researchSkills))
	}
	for _, skill := range researchSkills {
		if content, _ := skill["entrypoint_content"].(string); content == "" {
			t.Errorf("skill %s missing SKILL.md content", skill["id"])
		}
	}
	var trendResearch map[string]any
	for _, scannedSkill := range researchSkills {
		if scannedSkill["id"] == "trend-research" {
			trendResearch = scannedSkill
		}
	}
	if trendResearch == nil {
		t.Fatal("trend-research skill not discovered")
	}
	if got := trendResearch["description"]; got != "Collect platform and audience signals for a trend evaluation." {
		t.Errorf("English skill description mismatch: %q", got)
	}
	if got := trendResearch["description_zh"]; got != "收集平台与受众信号，形成可验证的趋势评估。" {
		t.Errorf("Chinese skill description mismatch: %q", got)
	}
	if content, _ := research["instructions_content"].(string); content == "" {
		t.Error("research-operator missing agent-detail.en.md content")
	}
	if content, _ := research["instructions_content_zh"].(string); !strings.Contains(content, "负责收集平台与受众信号") {
		t.Errorf("research-operator missing Chinese display instructions: %q", content)
	}
	if description, _ := research["description_zh"].(string); description != "收集平台与受众信号，形成可验证的趋势判断与视觉资产。" {
		t.Errorf("research-operator Chinese description mismatch: %q", description)
	}
	sourceFiles := research["source_files"].([]map[string]any)
	if len(sourceFiles) != 4 {
		t.Fatalf("research-operator want 4 linked source files, got %d: %+v", len(sourceFiles), sourceFiles)
	}
	sourceByPath := map[string]string{}
	for _, sourceFile := range sourceFiles {
		sourceByPath[sourceFile["path"].(string)] = sourceFile["content"].(string)
	}
	for _, expected := range []string{"agent-soul/PROFILE.yaml", "research-operator-skills/SKILL.md", "env/.env.example", "workdir/README.md"} {
		if sourceByPath[expected] == "" {
			t.Errorf("linked source file %s was not packaged", expected)
		}
	}
	if _, leaked := sourceByPath["env/.env"]; leaked {
		t.Fatal("real env/.env must never be packaged as a source file")
	}
	if content, _ := research["persona_content"].(string); content == "" {
		t.Error("research-operator missing agent-persona.html content")
	}

	// research-operator has 4 env declarations (from merged example+real).
	envDecls := research["env"].([]map[string]any)
	if len(envDecls) != 4 {
		t.Fatalf("research-operator want 4 env declarations, got %d", len(envDecls))
	}
	// PLATFORM_API_KEY should be flagged secret + configured + carry a digest.
	var apiKeyDecl map[string]any
	for _, d := range envDecls {
		if d["name"] == "PLATFORM_API_KEY" {
			apiKeyDecl = d
		}
	}
	if apiKeyDecl == nil {
		t.Fatal("PLATFORM_API_KEY declaration missing")
	}
	if !apiKeyDecl["configured"].(bool) {
		t.Error("PLATFORM_API_KEY should be configured (present in .env)")
	}
	if !apiKeyDecl["secret"].(bool) {
		t.Error("PLATFORM_API_KEY should be flagged secret")
	}
	if digest, ok := apiKeyDecl["value_digest"].(string); !ok || digest == "" {
		t.Errorf("PLATFORM_API_KEY should carry a non-empty value digest when key is supplied, got %v", apiKeyDecl["value_digest"])
	}

	// research-operator has an MCP declaration with 1 server.
	mcp := research["mcp"].(map[string]any)
	if !mcp["has_servers"].(bool) {
		t.Fatalf("research-operator mcp should have servers: %+v", mcp)
	}
	if count, _ := mcp["server_count"].(int); count != 1 {
		// Native manifest stores int; tolerate float64 from JSON round-trips.
		if fc, ok := mcp["server_count"].(float64); !ok || int(fc) != 1 {
			t.Fatalf("research-operator mcp server_count wrong: %+v", mcp)
		}
	}
	// The MCP ${PLATFORM_API_KEY}/${PLATFORM_API_BASE} refs are declared, so no
	// unresolved_env should be present.
	if mcp["unresolved_env"] != nil {
		t.Errorf("MCP refs should all be declared; got unresolved_env: %v", mcp["unresolved_env"])
	}
}

// TestScanDirectory_DirectoryHashStable confirms rescanning the same directory
// produces the same canonical hash (idempotent; map iteration order does not
// perturb it).
func TestScanDirectory_DirectoryHashStable(t *testing.T) {
	r1, err := ScanDirectory(t.Context(), fixtureRoot(t), nil)
	if err != nil {
		t.Fatalf("scan 1: %v", err)
	}
	r2, err := ScanDirectory(t.Context(), fixtureRoot(t), nil)
	if err != nil {
		t.Fatalf("scan 2: %v", err)
	}
	if r1.DirectoryHash != r2.DirectoryHash {
		t.Fatalf("directory hash non-deterministic: %q vs %q", r1.DirectoryHash, r2.DirectoryHash)
	}
	if !strings.HasPrefix(r1.DirectoryHash, "sha256:") {
		t.Errorf("directory hash must be sha256-prefixed: %q", r1.DirectoryHash)
	}
}

// TestScanDirectory_RejectsBadPath confirms the scanner reuses the daemon path
// validators: a forbidden root (home dir) is rejected.
func TestScanDirectory_RejectsBadPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot resolve home dir: %v", err)
	}
	if _, err := ScanDirectory(t.Context(), home, nil); err == nil {
		t.Fatal("scanner must reject the user's home directory")
	}
}

// TestScanDirectory_RejectsNonAgentWakerDir confirms the repository-discovery
// gate: a directory without capabilities/registry.yaml is rejected.
func TestScanDirectory_RejectsNonAgentWakerDir(t *testing.T) {
	tmp := t.TempDir()
	if _, err := ScanDirectory(t.Context(), tmp, nil); err == nil {
		t.Fatal("scanner must reject a directory without capabilities/registry.yaml")
	}
}

// TestScanDirectory_SkipsRuntimeDirs confirms .git, node_modules, and workdir
// contents do not enter the scan result. The fixture includes a marker file in
// each; its content must never appear.
func TestScanDirectory_SkipsRuntimeDirs(t *testing.T) {
	result, err := ScanDirectory(t.Context(), fixtureRoot(t), nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	blob, _ := json.Marshal(result)
	// node_modules/foo marker
	if strings.Contains(string(blob), `"name":"foo"`) {
		t.Errorf("node_modules content leaked into scan: %s", blob)
	}
	// workdir/run.yaml marker content
	if strings.Contains(string(blob), "2026-07-12-fixture") {
		t.Errorf("workdir content leaked into scan: %s", blob)
	}
}

// TestScanDirectory_BinaryAssetReportedSkipped confirms the PNG in
// research-operator/trend-research/cover.png is excluded from the skill bundle
// AND reported as a skipped-file diagnostic (never silently lost).
func TestScanDirectory_BinaryAssetReportedSkipped(t *testing.T) {
	result, err := ScanDirectory(t.Context(), fixtureRoot(t), nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	// The cover.png path must appear in a diagnostic (severity warning, code
	// file_skipped). It must NOT appear in any skill's content.
	blob, _ := json.Marshal(result)
	if !strings.Contains(string(blob), "cover.png") {
		t.Errorf("binary asset cover.png was silently dropped (no diagnostic): %s", blob)
	}
	// And the raw PNG signature must not be in the manifest.
	if strings.Contains(string(blob), "PNG") && strings.Contains(string(blob), "IHDR") {
		t.Errorf("binary PNG content leaked into manifest")
	}
}

func TestScanRoleSkills_IgnoresSupportDirsAndGeneratedCaches(t *testing.T) {
	root := t.TempDir()
	skillsDir := filepath.Join(root, "example-role", "example-role-skills")
	mustWrite := func(relPath, content string) {
		t.Helper()
		path := filepath.Join(skillsDir, relPath)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", relPath, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", relPath, err)
		}
	}

	// Shared role-level scripts are support files for the meta skill, not a
	// specialist skill directory and therefore do not require scripts/SKILL.md.
	mustWrite("scripts/validate-role.rb", "puts 'ok'")
	// A real specialist skill may contain source scripts, but generated Python
	// caches must not enter its bundle or produce noisy skipped-file warnings.
	mustWrite("format-article/SKILL.md", "# Format article")
	mustWrite("format-article/scripts/render.py", "print('ok')")
	mustWrite("format-article/scripts/__pycache__/render.cpython-312.pyc", "binary-cache")

	profile := agentwaker.ProfileV2{ID: "example-role", DisplayName: "Example Role"}
	skills, diags, err := scanRoleSkills(root, "example-role", skillsDir, profile)
	if err != nil {
		t.Fatalf("scanRoleSkills: %v", err)
	}
	if len(diags) != 0 {
		t.Fatalf("support dirs and generated caches should be ignored, got diagnostics: %+v", diags)
	}
	if len(skills) != 1 {
		t.Fatalf("want only the real specialist skill, got %d: %+v", len(skills), skills)
	}
	if skills[0]["id"] != "format-article" {
		t.Fatalf("wrong skill discovered: %+v", skills[0])
	}
	if skills[0]["file_count"] != 1 {
		t.Fatalf("want only render.py as supporting file, got %+v", skills[0]["file_count"])
	}
}

// walkEnvDecls walks role.env[] and calls fn for each declaration. Handles both
// native map[string]any manifests and JSON-round-tripped manifests.
func walkEnvDecls(manifest any, fn func(decl map[string]any, role string)) {
	m, ok := manifest.(map[string]any)
	if !ok {
		return
	}
	rolesAny, ok := m["roles"]
	if !ok {
		return
	}
	var roles []map[string]any
	switch r := rolesAny.(type) {
	case []map[string]any:
		roles = r
	case []any:
		for _, x := range r {
			if rm, ok := x.(map[string]any); ok {
				roles = append(roles, rm)
			}
		}
	}
	for _, rm := range roles {
		roleID, _ := rm["id"].(string)
		var envs []map[string]any
		switch e := rm["env"].(type) {
		case []map[string]any:
			envs = e
		case []any:
			for _, x := range e {
				if em, ok := x.(map[string]any); ok {
					envs = append(envs, em)
				}
			}
		}
		for _, em := range envs {
			fn(em, roleID)
		}
	}
}
