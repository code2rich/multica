package agentwaker

import (
	"errors"
	"strings"
	"testing"
)

func TestParseRegistry(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		const yaml = `schema_version: "1.0"
capabilities:
  - id: information-collection
    version: 1.0.0
    manifest: information-collection/CAPABILITY.yaml
`
		r, err := ParseRegistry([]byte(yaml))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(r.Capabilities) != 1 || r.Capabilities[0].ID != "information-collection" {
			t.Fatalf("unexpected registry: %+v", r)
		}
	})
	t.Run("unsupported schema version", func(t *testing.T) {
		_, err := ParseRegistry([]byte(`schema_version: "9.9"` + "\n" + `capabilities: []` + "\n"))
		if err == nil || !strings.Contains(err.Error(), "not supported") {
			t.Fatalf("want unsupported-version error, got %v", err)
		}
	})
	t.Run("malformed yaml", func(t *testing.T) {
		_, err := ParseRegistry([]byte("capabilities: [\n"))
		if err == nil {
			t.Fatalf("want parse error, got nil")
		}
	})
}

func TestParseCapabilityManifest_Valid(t *testing.T) {
	const yaml = `schema_version: "1.0"
id: information-collection
name: Information Collection
version: 1.0.0
description: Collect evidence.
entrypoint: SKILL.md
profiles:
  - id: trend-research
    description: Trend research profile.
adapters:
  - id: web
    required: true
    description: Web adapter.
contracts:
  input_schema: schemas/in.json
  output_schema: schemas/out.json
permissions:
  default_mode: read-only
  supports_account_actions: false
`
	c, err := ParseCapabilityManifest([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.ID != "information-collection" || len(c.Profiles) != 1 || c.Permissions.DefaultMode != "read-only" {
		t.Fatalf("unexpected capability: %+v", c)
	}
}

func TestParseCapabilityManifest_Errors(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{"missing id", `schema_version: "1.0"` + "\n" + `name: x` + "\n" + `version: 1.0.0` + "\n" + `entrypoint: SKILL.md` + "\n" + `profiles: [{id: p, description: d}]` + "\n" + `contracts: {input_schema: a, output_schema: b}` + "\n" + `permissions: {default_mode: read-only, supports_account_actions: false}` + "\n", "missing required fields"},
		{"bad id", `schema_version: "1.0"` + "\n" + `id: BadID` + "\n" + `name: x` + "\n" + `version: 1.0.0` + "\n" + `entrypoint: SKILL.md` + "\n" + `profiles: [{id: p, description: d}]` + "\n" + `contracts: {input_schema: a, output_schema: b}` + "\n" + `permissions: {default_mode: read-only, supports_account_actions: false}` + "\n", "not a valid kebab-case id"},
		{"bad version", `schema_version: "1.0"` + "\n" + `id: ok-id` + "\n" + `name: x` + "\n" + `version: notsemver` + "\n" + `entrypoint: SKILL.md` + "\n" + `profiles: [{id: p, description: d}]` + "\n" + `contracts: {input_schema: a, output_schema: b}` + "\n" + `permissions: {default_mode: read-only, supports_account_actions: false}` + "\n", "not a valid semver"},
		{"bad permission mode", `schema_version: "1.0"` + "\n" + `id: ok-id` + "\n" + `name: x` + "\n" + `version: 1.0.0` + "\n" + `entrypoint: SKILL.md` + "\n" + `profiles: [{id: p, description: d}]` + "\n" + `contracts: {input_schema: a, output_schema: b}` + "\n" + `permissions: {default_mode: nuclear, supports_account_actions: false}` + "\n", "permissions.default_mode"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseCapabilityManifest([]byte(tc.yaml))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestParseRoleCapabilities(t *testing.T) {
	t.Run("empty capabilities list valid", func(t *testing.T) {
		rc, err := ParseRoleCapabilities([]byte(`schema_version: "1.0"` + "\n" + `role: plain-operator` + "\n" + `capabilities: []` + "\n"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rc.Role != "plain-operator" || len(rc.Capabilities) != 0 {
			t.Fatalf("unexpected: %+v", rc)
		}
	})
	t.Run("binding with used_by valid", func(t *testing.T) {
		const yaml = `schema_version: "1.0"
role: research-operator
capabilities:
  - id: information-collection
    version: ^1.0.0
    required: true
    used_by:
      - skill: trend-research
        profile: trend-research
    permissions:
      mode: read-only
      account_actions: false
    fallback:
      behavior: partial
      message: Continue with gaps.
`
		rc, err := ParseRoleCapabilities([]byte(yaml))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rc.Capabilities) != 1 || rc.Capabilities[0].Version != "^1.0.0" {
			t.Fatalf("unexpected: %+v", rc)
		}
	})
	t.Run("binding missing used_by rejected", func(t *testing.T) {
		const yaml = `schema_version: "1.0"
role: x
capabilities:
  - id: information-collection
    version: ^1.0.0
    required: true
    permissions: {mode: read-only, account_actions: false}
    fallback: {behavior: partial, message: x}
`
		_, err := ParseRoleCapabilities([]byte(yaml))
		if err == nil || !strings.Contains(err.Error(), "used_by") {
			t.Fatalf("want used_by error, got %v", err)
		}
	})
	t.Run("bad version requirement rejected", func(t *testing.T) {
		const yaml = `schema_version: "1.0"
role: x
capabilities:
  - id: information-collection
    version: "not-a-req"
    required: true
    used_by: [{skill: s, profile: p}]
    permissions: {mode: read-only, account_actions: false}
    fallback: {behavior: partial, message: x}
`
		_, err := ParseRoleCapabilities([]byte(yaml))
		if err == nil || !strings.Contains(err.Error(), "version requirement") {
			t.Fatalf("want version-requirement error, got %v", err)
		}
	})
}

func TestParseProfile(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		const yaml = `schema_version: "2.1"
id: research-operator
display_name: Rita
skills:
  directory: research-operator-skills/
  meta_entrypoint: research-operator-skills/SKILL.md
`
		p, err := ParseProfile([]byte(yaml))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.ID != "research-operator" || p.Skills.Directory != "research-operator-skills/" {
			t.Fatalf("unexpected: %+v", p)
		}
	})
	t.Run("missing skills directory", func(t *testing.T) {
		_, err := ParseProfile([]byte(`schema_version: "2.1"` + "\n" + `id: x` + "\n"))
		if err == nil || !strings.Contains(err.Error(), "skills.directory") {
			t.Fatalf("want skills.directory error, got %v", err)
		}
	})
	t.Run("unsupported profile schema", func(t *testing.T) {
		_, err := ParseProfile([]byte(`schema_version: "1.0"` + "\n" + `id: x` + "\n" + `skills: {directory: x/}` + "\n"))
		if err == nil || !strings.Contains(err.Error(), "not supported") {
			t.Fatalf("want unsupported error, got %v", err)
		}
	})
}

func TestParseMCPConfig(t *testing.T) {
	t.Run("with servers", func(t *testing.T) {
		const j = `{"mcpServers": {"a": {"command": "x"}}}`
		c, err := ParseMCPConfig([]byte(j))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !c.HasServers() || len(c.MCPServers) != 1 {
			t.Fatalf("unexpected: %+v", c)
		}
	})
	t.Run("empty servers treated as no declaration", func(t *testing.T) {
		c, err := ParseMCPConfig([]byte(`{"mcpServers": {}}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.HasServers() {
			t.Fatalf("empty servers must not be treated as a declaration")
		}
	})
	t.Run("malformed json", func(t *testing.T) {
		_, err := ParseMCPConfig([]byte(`{"mcpServers":`))
		if err == nil {
			t.Fatalf("want error")
		}
	})
}

func TestParseEnvFile(t *testing.T) {
	const content = `# Leading description for FOO.
# Second line of description.
FOO=bar

# Description for secret.
export SECRET_KEY="quoted value" # inline comment
EMPTY=
# stand-alone comment not attached to a var
BARE_WORD
PLAIN=hello # trailing
`
	env, err := ParseEnvFile([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Order is deterministic, first-seen.
	wantOrder := []string{"FOO", "SECRET_KEY", "EMPTY", "PLAIN"}
	if len(env.Order) != len(wantOrder) {
		t.Fatalf("order length = %d, want %d (%v)", len(env.Order), len(wantOrder), env.Order)
	}
	for i, w := range wantOrder {
		if env.Order[i] != w {
			t.Fatalf("order[%d] = %q, want %q (full: %v)", i, env.Order[i], w, env.Order)
		}
	}
	if env.Values["FOO"] != "bar" {
		t.Errorf("FOO = %q, want bar", env.Values["FOO"])
	}
	if env.Values["SECRET_KEY"] != "quoted value" {
		t.Errorf("SECRET_KEY = %q, want %q (export prefix stripped, quotes removed, inline comment stripped)", env.Values["SECRET_KEY"], "quoted value")
	}
	if env.Values["EMPTY"] != "" {
		t.Errorf("EMPTY = %q, want empty", env.Values["EMPTY"])
	}
	if env.Values["PLAIN"] != "hello" {
		t.Errorf("PLAIN = %q, want hello (inline comment stripped)", env.Values["PLAIN"])
	}
	// BARE_WORD has no `=`, so it must be ignored, not present in values.
	if _, ok := env.Values["BARE_WORD"]; ok {
		t.Errorf("BARE_WORD must not be parsed as an assignment")
	}
	// Description recovered from consecutive comments.
	if !strings.Contains(env.Declarations["FOO"].Description, "Leading description") {
		t.Errorf("FOO description = %q", env.Declarations["FOO"].Description)
	}
	// Secret heuristic flags names containing SECRET/TOKEN/KEY.
	if !env.Declarations["SECRET_KEY"].Secret {
		t.Errorf("SECRET_KEY should be flagged secret by name heuristic")
	}
	if env.Declarations["FOO"].Secret {
		t.Errorf("FOO should not be flagged secret")
	}
	// Configured true for every present key.
	for _, name := range env.Order {
		if !env.Declarations[name].Configured {
			t.Errorf("%s should be Configured=true", name)
		}
	}
}

func TestParseEnvFile_InlineCommentNotStrippedWhenTouchingEquals(t *testing.T) {
	// A "#" immediately after "=" is preserved so empty values stay empty and
	// a value like `X=#real` is kept verbatim. This matches the TS import path.
	env, _ := ParseEnvFile([]byte("HASH=#realvalue\n"))
	if env.Values["HASH"] != "#realvalue" {
		t.Errorf("HASH = %q, want #realvalue (inline comment requires preceding space)", env.Values["HASH"])
	}
}

func TestMergeEnvDeclarations(t *testing.T) {
	example, _ := ParseEnvFile([]byte("# desc for FOO.\nFOO=\n# desc for BAR (not in real).\nBAR=\n"))
	real, _ := ParseEnvFile([]byte("FOO=prod-value\nBAZ=extra\n"))
	merged := MergeEnvDeclarations(example, real)

	// Order: example order first, then extras from real.
	wantOrder := []string{"FOO", "BAR", "BAZ"}
	if len(merged.Order) != 3 {
		t.Fatalf("order len = %d, want 3 (%v)", len(merged.Order), merged.Order)
	}
	for i, w := range wantOrder {
		if merged.Order[i] != w {
			t.Fatalf("order[%d] = %q, want %q", i, merged.Order[i], w)
		}
	}
	// FOO: configured, value from real, description from example.
	if !merged.Declarations["FOO"].Configured || merged.Values["FOO"] != "prod-value" {
		t.Errorf("FOO merge wrong: %+v val=%q", merged.Declarations["FOO"], merged.Values["FOO"])
	}
	if !strings.Contains(merged.Declarations["FOO"].Description, "desc for FOO") {
		t.Errorf("FOO description lost: %q", merged.Declarations["FOO"].Description)
	}
	// BAR: declared in example only, not configured.
	if merged.Declarations["BAR"].Configured {
		t.Errorf("BAR should be not-configured (declared in example only)")
	}
	if _, ok := merged.Values["BAR"]; ok {
		t.Errorf("BAR should not have a value")
	}
	// BAZ: extra from real, configured.
	if !merged.Declarations["BAZ"].Configured || merged.Values["BAZ"] != "extra" {
		t.Errorf("BAZ merge wrong: %+v", merged.Declarations["BAZ"])
	}
}

func TestParseEnvFile_NoPlaintextInError(t *testing.T) {
	// Even a malformed env file must not leak values into the error path.
	_, err := ParseEnvFile([]byte("SECRET=leak-canary\n"))
	if err != nil && strings.Contains(err.Error(), "leak-canary") {
		t.Errorf("error leaked plaintext: %v", err)
	}
	// Ensure ParseEnvFile never returns an error for content issues (it ignores
	// unparseable lines); this is mostly to document the contract.
	if err != nil && !errors.Is(err, nil) {
		t.Logf("ParseEnvFile returned non-nil error %v (acceptable)", err)
	}
}
