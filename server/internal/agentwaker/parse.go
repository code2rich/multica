package agentwaker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var promptFileRE = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*\.prompt\.md$`)

// ParseDailyAutomationManifest parses and structurally validates one role's
// daily automation contract. Filesystem, cron, timezone, and title-template
// validation remain at the daemon boundary where their dependencies live.
func ParseDailyAutomationManifest(data []byte, expectedRoleID string) (DailyAutomationManifest, error) {
	var manifest DailyAutomationManifest
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&manifest); err != nil {
		return DailyAutomationManifest{}, fmt.Errorf("agentwaker: parse daily-tasks/manifest.yaml: %w", err)
	}
	if manifest.SchemaVersion != AutomationSchemaVersion {
		return DailyAutomationManifest{}, fmt.Errorf("agentwaker: automation schema_version %q not supported (want %q)", manifest.SchemaVersion, AutomationSchemaVersion)
	}
	if manifest.RoleID != expectedRoleID {
		return DailyAutomationManifest{}, fmt.Errorf("agentwaker: automation role_id %q does not match profile id %q", manifest.RoleID, expectedRoleID)
	}
	if len(manifest.Automations) == 0 || len(manifest.Automations) > 32 {
		return DailyAutomationManifest{}, fmt.Errorf("agentwaker: role %q must declare 1..32 automations", expectedRoleID)
	}
	ids := make(map[string]bool, len(manifest.Automations))
	prompts := make(map[string]bool, len(manifest.Automations))
	for i, automation := range manifest.Automations {
		prefix := fmt.Sprintf("agentwaker: automation[%d]", i)
		if !isValidID(automation.ID) {
			return DailyAutomationManifest{}, fmt.Errorf("%s id %q is not a valid kebab-case id", prefix, automation.ID)
		}
		if ids[automation.ID] {
			return DailyAutomationManifest{}, fmt.Errorf("%s duplicate id %q", prefix, automation.ID)
		}
		ids[automation.ID] = true
		if strings.TrimSpace(automation.Title) == "" {
			return DailyAutomationManifest{}, fmt.Errorf("%s title is empty", prefix)
		}
		if !promptFileRE.MatchString(automation.PromptFile) {
			return DailyAutomationManifest{}, fmt.Errorf("%s prompt_file %q is invalid", prefix, automation.PromptFile)
		}
		if prompts[automation.PromptFile] {
			return DailyAutomationManifest{}, fmt.Errorf("%s duplicate prompt_file %q", prefix, automation.PromptFile)
		}
		prompts[automation.PromptFile] = true
		switch automation.Execution.Mode {
		case "run_only":
			if automation.Execution.IssueTitleTemplate != "" {
				return DailyAutomationManifest{}, fmt.Errorf("%s issue_title_template is forbidden for run_only", prefix)
			}
		case "create_issue":
			if strings.TrimSpace(automation.Execution.IssueTitleTemplate) == "" {
				return DailyAutomationManifest{}, fmt.Errorf("%s issue_title_template is required for create_issue", prefix)
			}
		default:
			return DailyAutomationManifest{}, fmt.Errorf("%s execution mode %q is invalid", prefix, automation.Execution.Mode)
		}
		if automation.Schedule.Kind != "cron" || strings.TrimSpace(automation.Schedule.Expression) == "" || strings.TrimSpace(automation.Schedule.Timezone) == "" {
			return DailyAutomationManifest{}, fmt.Errorf("%s schedule must declare cron expression and timezone", prefix)
		}
		if automation.Schedule.InitialEnabled {
			return DailyAutomationManifest{}, fmt.Errorf("%s initial_enabled must be false", prefix)
		}
		if automation.Sync.Content != "source-authoritative" || automation.Sync.Schedule != "source-authoritative" || automation.Sync.Activation != "workspace-preserve" || automation.Sync.Missing != "archive" {
			return DailyAutomationManifest{}, fmt.Errorf("%s sync policy is invalid", prefix)
		}
		if automation.Governance.ExternalWrites != "read-only" && automation.Governance.ExternalWrites != "approval-required" {
			return DailyAutomationManifest{}, fmt.Errorf("%s governance.external_writes %q is invalid", prefix, automation.Governance.ExternalWrites)
		}
	}
	return manifest, nil
}

// ParseRegistry parses capabilities/registry.yaml.
func ParseRegistry(data []byte) (Registry, error) {
	var r Registry
	if err := yaml.Unmarshal(data, &r); err != nil {
		return Registry{}, fmt.Errorf("agentwaker: parse registry.yaml: %w", err)
	}
	if r.SchemaVersion == "" {
		r.SchemaVersion = RegistrySchemaVersion
	}
	if r.SchemaVersion != RegistrySchemaVersion {
		return Registry{}, fmt.Errorf("agentwaker: registry schema_version %q not supported (want %q)", r.SchemaVersion, RegistrySchemaVersion)
	}
	return r, nil
}

// ParseCapabilityManifest parses capabilities/{id}/CAPABILITY.yaml and applies
// the structural checks required for the capability to be importable.
func ParseCapabilityManifest(data []byte) (CapabilityManifest, error) {
	var c CapabilityManifest
	if err := yaml.Unmarshal(data, &c); err != nil {
		return CapabilityManifest{}, fmt.Errorf("agentwaker: parse CAPABILITY.yaml: %w", err)
	}
	if c.SchemaVersion == "" {
		c.SchemaVersion = CapabilitySchemaVersion
	}
	if c.SchemaVersion != CapabilitySchemaVersion {
		return CapabilityManifest{}, fmt.Errorf("agentwaker: capability %q schema_version %q not supported (want %q)", c.ID, c.SchemaVersion, CapabilitySchemaVersion)
	}
	var missing []string
	if c.ID == "" {
		missing = append(missing, "id")
	}
	if c.Name == "" {
		missing = append(missing, "name")
	}
	if c.Version == "" {
		missing = append(missing, "version")
	}
	if c.Entrypoint == "" {
		missing = append(missing, "entrypoint")
	}
	if len(c.Profiles) == 0 {
		missing = append(missing, "profiles")
	}
	if c.Contracts.InputSchema == "" || c.Contracts.OutputSchema == "" {
		missing = append(missing, "contracts.input_schema/output_schema")
	}
	if c.Permissions.DefaultMode == "" {
		missing = append(missing, "permissions.default_mode")
	}
	if len(missing) > 0 {
		return CapabilityManifest{}, fmt.Errorf("agentwaker: capability %q missing required fields: %s", c.ID, strings.Join(missing, ", "))
	}
	if !isValidID(c.ID) {
		return CapabilityManifest{}, fmt.Errorf("agentwaker: capability id %q is not a valid kebab-case id", c.ID)
	}
	if !isValidVersion(c.Version) {
		return CapabilityManifest{}, fmt.Errorf("agentwaker: capability %q version %q is not a valid semver", c.ID, c.Version)
	}
	if !isValidPermissionMode(c.Permissions.DefaultMode) {
		return CapabilityManifest{}, fmt.Errorf("agentwaker: capability %q permissions.default_mode %q invalid", c.ID, c.Permissions.DefaultMode)
	}
	return c, nil
}

// ParseRoleCapabilities parses */capabilities.yaml. An empty capability list is
// valid (most roles declare no shared-capability dependencies).
func ParseRoleCapabilities(data []byte) (RoleCapabilities, error) {
	var rc RoleCapabilities
	if err := yaml.Unmarshal(data, &rc); err != nil {
		return RoleCapabilities{}, fmt.Errorf("agentwaker: parse capabilities.yaml: %w", err)
	}
	if rc.SchemaVersion == "" {
		rc.SchemaVersion = RoleCapabilitiesVersion
	}
	if rc.SchemaVersion != RoleCapabilitiesVersion {
		return RoleCapabilities{}, fmt.Errorf("agentwaker: role-capabilities schema_version %q not supported (want %q)", rc.SchemaVersion, RoleCapabilitiesVersion)
	}
	if rc.Role == "" {
		return RoleCapabilities{}, errors.New("agentwaker: capabilities.yaml missing role")
	}
	for _, b := range rc.Capabilities {
		if b.ID == "" {
			return RoleCapabilities{}, errors.New("agentwaker: capability binding missing id")
		}
		if b.Version == "" {
			return RoleCapabilities{}, fmt.Errorf("agentwaker: capability %q missing version", b.ID)
		}
		if !isValidVersionRequirement(b.Version) {
			return RoleCapabilities{}, fmt.Errorf("agentwaker: capability %q version requirement %q invalid", b.ID, b.Version)
		}
		if len(b.UsedBy) == 0 {
			return RoleCapabilities{}, fmt.Errorf("agentwaker: capability %q missing used_by", b.ID)
		}
		for _, u := range b.UsedBy {
			if u.Skill == "" || u.Profile == "" {
				return RoleCapabilities{}, fmt.Errorf("agentwaker: capability %q has used_by entry missing skill/profile", b.ID)
			}
		}
		if !isValidPermissionMode(b.Permissions.Mode) {
			return RoleCapabilities{}, fmt.Errorf("agentwaker: capability %q permissions.mode %q invalid", b.ID, b.Permissions.Mode)
		}
		switch b.Fallback.Behavior {
		case "continue", "partial", "blocked":
		default:
			return RoleCapabilities{}, fmt.Errorf("agentwaker: capability %q fallback.behavior %q invalid", b.ID, b.Fallback.Behavior)
		}
		if b.Fallback.Message == "" {
			return RoleCapabilities{}, fmt.Errorf("agentwaker: capability %q fallback.message empty", b.ID)
		}
	}
	return rc, nil
}

// ParseProfile parses agent-soul/PROFILE.yaml, returning only the fields the
// integration needs. Unknown fields are ignored.
func ParseProfile(data []byte) (ProfileV2, error) {
	var p ProfileV2
	if err := yaml.Unmarshal(data, &p); err != nil {
		return ProfileV2{}, fmt.Errorf("agentwaker: parse PROFILE.yaml: %w", err)
	}
	if p.SchemaVersion == "" {
		p.SchemaVersion = ProfileSchemaVersion
	}
	if p.SchemaVersion != ProfileSchemaVersion {
		return ProfileV2{}, fmt.Errorf("agentwaker: profile schema_version %q not supported (want %q)", p.SchemaVersion, ProfileSchemaVersion)
	}
	if p.ID == "" {
		return ProfileV2{}, errors.New("agentwaker: PROFILE.yaml missing id")
	}
	if p.Skills.Directory == "" {
		return ProfileV2{}, errors.New("agentwaker: PROFILE.yaml missing skills.directory")
	}
	return p, nil
}

// ParseMCPConfig parses mcp/mcp.json. Returns a zero-value MCPConfig and nil
// error when the servers map is empty, so callers can distinguish "declared
// but empty" (no-op) from "absent" (caller decides). The HasServers helper
// encodes that distinction.
func ParseMCPConfig(data []byte) (MCPConfig, error) {
	var c MCPConfig
	// mcp.json is JSON; use encoding/json so ${ENV} interpolation and nesting
	// are preserved verbatim for later validation against declared env keys.
	if err := json.Unmarshal(data, &c); err != nil {
		return MCPConfig{}, fmt.Errorf("agentwaker: parse mcp.json: %w", err)
	}
	return c, nil
}

// HasServers reports whether the config declares at least one MCP server.
// An empty {"mcpServers":{}} is treated as no declaration so it cannot wipe a
// runtime default — matching the browser import behavior.
func (c MCPConfig) HasServers() bool {
	return len(c.MCPServers) > 0
}

// envLineRE mirrors the parser in packages/views/agents/lib/agent-import.ts:
// optional leading "export ", then NAME = value. Captures name and raw value.
var envLineRE = regexp.MustCompile(`^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$`)

// secretNameRE identifies variable names that look like secrets so previews
// can mark them accordingly without ever reading the value.
var secretNameRE = regexp.MustCompile(`(?i)(secret|token|key|password|credential|access_token)`)

// ParseEnvFile parses an env file (either .env.example or .env) into an
// EnvFile. For .env.example, values are placeholders and Descriptions are
// recovered from leading comment lines. For .env, Values holds the configured
// values and Configured is true for every present key.
//
// The parsing rules (ported from the TS import path):
//   - blank lines and lines whose first non-space char is "#" are comments;
//   - a comment line immediately preceding a declaration becomes its description;
//   - inline trailing comments are stripped at " #" (a "#" touching "=" is kept
//     so empty values remain empty);
//   - values wrapped in matching single or double quotes are unquoted.
//
// Values are retained here only because this runs on the daemon, over the exact
// {role}/env/.env path, and only for long enough to serve the apply request.
// They must pass through SanitizeEnvForPreview before entering any preview,
// snapshot, plan, log, event, or ordinary API response.
func ParseEnvFile(data []byte) (EnvFile, error) {
	out := EnvFile{
		Declarations: make(map[string]EnvDeclaration),
		Values:       make(map[string]string),
	}
	var pendingDesc string
	lines := bytes.Split(data, []byte("\n"))
	for _, raw := range lines {
		line := strings.TrimRight(string(raw), "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			pendingDesc = ""
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			// Accumulate consecutive comment lines as a description block.
			comment := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
			if pendingDesc == "" {
				pendingDesc = comment
			} else {
				pendingDesc += "\n" + comment
			}
			continue
		}
		m := envLineRE.FindStringSubmatch(line)
		if m == nil {
			// Non-blank, non-comment, non-assignment: ignore but do not fail.
			pendingDesc = ""
			continue
		}
		name, rawValue := m[1], m[2]
		value := stripInlineComment(rawValue)
		value = unquote(value)
		if _, exists := out.Declarations[name]; !exists {
			out.Order = append(out.Order, name)
		}
		decl := out.Declarations[name]
		decl.Name = name
		decl.Description = pendingDesc
		decl.Secret = secretNameRE.MatchString(name)
		// Configured is set true when a value is present in .env. For
		// .env.example the caller will merge this against the real .env later.
		decl.Configured = true
		out.Declarations[name] = decl
		out.Values[name] = value
		pendingDesc = ""
	}
	return out, nil
}

// stripInlineComment removes a trailing " #..." comment, preserving a "#" that
// immediately follows "=" (empty value) and any "#" inside quotes.
func stripInlineComment(value string) string {
	inSingle, inDouble := false, false
	for i := 0; i < len(value); i++ {
		c := value[i]
		switch c {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble && i > 0 && value[i-1] == ' ' {
				return strings.TrimRight(value[:i], " \t")
			}
		}
	}
	return value
}

// unquote removes a single layer of matching surrounding quotes.
func unquote(value string) string {
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

// MergeEnvDeclarations folds .env.example declarations into the parsed .env
// result: the example supplies description/required hints for keys the real
// file also declares, and reveals keys declared only in the example (which are
// then marked not-configured). The result is the union; values come from the
// receiver (the real .env) only.
func MergeEnvDeclarations(example, real EnvFile) EnvFile {
	merged := EnvFile{
		Declarations: make(map[string]EnvDeclaration),
		Values:       make(map[string]string),
	}
	// Preserve deterministic order: example order first, then any extras.
	seen := make(map[string]bool)
	for _, name := range example.Order {
		seen[name] = true
		merged.Order = append(merged.Order, name)
		decl := example.Declarations[name]
		decl.Name = name
		if v, ok := real.Values[name]; ok {
			decl.Configured = true
			merged.Values[name] = v
		} else {
			decl.Configured = false
		}
		merged.Declarations[name] = decl
	}
	for _, name := range real.Order {
		if seen[name] {
			continue
		}
		seen[name] = true
		merged.Order = append(merged.Order, name)
		decl := real.Declarations[name]
		decl.Configured = true
		merged.Declarations[name] = decl
		merged.Values[name] = real.Values[name]
	}
	return merged
}

// --- validation helpers (mirror the JSON schemas in agentwaker/schemas/) ---

var (
	idPatternRE  = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	semverRE     = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
	versionReqRE = regexp.MustCompile(`^(?:[~^]|>=?)?[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)
)

func isValidID(s string) bool                 { return idPatternRE.MatchString(s) }
func isValidVersion(s string) bool            { return semverRE.MatchString(s) }
func isValidVersionRequirement(s string) bool { return versionReqRE.MatchString(s) }
func isValidPermissionMode(s string) bool {
	switch s {
	case "read-only", "local-write", "external-write":
		return true
	}
	return false
}
