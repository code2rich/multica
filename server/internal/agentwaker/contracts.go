// Package agentwaker contains the pure parsing, hashing, and redaction
// primitives for the AgentWaker directory integration.
//
// It understands AgentWaker Schema 2.1 plus the shared-capability manifest
// schema (1.0). The package performs no filesystem I/O and no database access;
// callers (the daemon scanner, the server apply path) feed it bytes and receive
// typed values. Plaintext environment values never leave this package's
// sanitization boundary in a form that can be persisted or displayed.
package agentwaker

// SchemaVersion constants mirror the contract versions published by AgentWaker.
const (
	CapabilitySchemaVersion  = "1.0" // CAPABILITY.yaml, registry.yaml
	RoleCapabilitiesVersion  = "1.0" // */capabilities.yaml
	ProfileSchemaVersion     = "2.1" // agent-soul/PROFILE.yaml
	RegistrySchemaVersion    = "1.0" // capabilities/registry.yaml
	LockSchemaVersion        = "1.0" // multica-side lock representation
)

// Registry mirrors capabilities/registry.yaml. It is the directory-level index
// of available shared capabilities and their current manifest version.
type Registry struct {
	SchemaVersion string             `yaml:"schema_version" json:"schema_version"`
	Capabilities  []RegistryCapability `yaml:"capabilities" json:"capabilities"`
}

// RegistryCapability is one entry in the registry pointing at a capability
// manifest file relative to the repository root.
type RegistryCapability struct {
	ID      string `yaml:"id" json:"id"`
	Version string `yaml:"version" json:"version"`
	Manifest string `yaml:"manifest" json:"manifest"` // relative path to CAPABILITY.yaml
}

// CapabilityManifest mirrors capabilities/{id}/CAPABILITY.yaml. Identity in a
// workspace is (source_id, capability_id); version and content hash are
// separate, continuously-synchronized fields.
type CapabilityManifest struct {
	SchemaVersion string                 `yaml:"schema_version" json:"schema_version"`
	ID            string                 `yaml:"id" json:"id"`
	Name          string                 `yaml:"name" json:"name"`
	Version       string                 `yaml:"version" json:"version"`
	Description   string                 `yaml:"description" json:"description"`
	Entrypoint    string                 `yaml:"entrypoint" json:"entrypoint"`
	Profiles      []CapabilityProfile    `yaml:"profiles" json:"profiles"`
	Adapters      []CapabilityAdapter    `yaml:"adapters" json:"adapters"`
	Contracts     CapabilityContracts    `yaml:"contracts" json:"contracts"`
	Requires      CapabilityRequires     `yaml:"requires,omitempty" json:"requires,omitempty"`
	Permissions   CapabilityPermissions  `yaml:"permissions" json:"permissions"`
}

// CapabilityProfile is a named, selectable mode of using a capability.
type CapabilityProfile struct {
	ID          string `yaml:"id" json:"id"`
	Description string `yaml:"description" json:"description"`
}

// CapabilityAdapter declares an upstream integration surface the capability
// may use. Required adapters are activation blockers when unavailable.
type CapabilityAdapter struct {
	ID          string `yaml:"id" json:"id"`
	Required    bool   `yaml:"required" json:"required"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// CapabilityContracts names the input/output JSON schemas (paths relative to
// the capability package root).
type CapabilityContracts struct {
	InputSchema  string `yaml:"input_schema" json:"input_schema"`
	OutputSchema string `yaml:"output_schema" json:"output_schema"`
}

// CapabilityRequires is the optional environment/MCP dependency list.
type CapabilityRequires struct {
	Environment []string `yaml:"environment,omitempty" json:"environment,omitempty"`
	MCP         []string `yaml:"mcp,omitempty" json:"mcp,omitempty"`
}

// CapabilityPermissions is the system policy baseline. Effective permissions
// are the intersection of this, the role declaration, and current user
// approval; import rejects attempted expansion rather than silently clamping.
type CapabilityPermissions struct {
	DefaultMode           string `yaml:"default_mode" json:"default_mode"` // read-only | local-write | external-write
	SupportsAccountActions bool  `yaml:"supports_account_actions" json:"supports_account_actions"`
}

// RoleCapabilities mirrors */capabilities.yaml — the role-side dependency
// manifest binding role-owned skills to shared-capability profiles.
type RoleCapabilities struct {
	SchemaVersion string                  `yaml:"schema_version" json:"schema_version"`
	Role          string                  `yaml:"role" json:"role"`
	Capabilities  []RoleCapabilityBinding `yaml:"capabilities" json:"capabilities"`
}

// RoleCapabilityBinding is one role → capability dependency.
type RoleCapabilityBinding struct {
	ID                string                       `yaml:"id" json:"id"`
	Version           string                       `yaml:"version" json:"version"`           // e.g. "^1.0.0"
	Required          bool                         `yaml:"required" json:"required"`
	UsedBy            []RoleCapabilityUse          `yaml:"used_by" json:"used_by"`
	Permissions       RoleCapabilityPermissions    `yaml:"permissions" json:"permissions"`
	Fallback          RoleCapabilityFallback       `yaml:"fallback" json:"fallback"`
}

// RoleCapabilityUse ties one role-owned skill to one capability profile.
type RoleCapabilityUse struct {
	Skill   string `yaml:"skill" json:"skill"`
	Profile string `yaml:"profile" json:"profile"`
}

// RoleCapabilityPermissions is the role's requested permission envelope.
type RoleCapabilityPermissions struct {
	Mode           string `yaml:"mode" json:"mode"` // read-only | local-write | external-write
	AccountActions bool   `yaml:"account_actions" json:"account_actions"`
}

// RoleCapabilityFallback declares behavior when the capability is unavailable.
type RoleCapabilityFallback struct {
	Behavior string `yaml:"behavior" json:"behavior"` // continue | partial | blocked
	Message  string `yaml:"message" json:"message"`
}

// ProfileV2 is the subset of agent-soul/PROFILE.yaml the integration reads.
// The full profile is large; only identity, routing metadata, and the skill
// directory declaration are needed to import a role.
type ProfileV2 struct {
	SchemaVersion string         `yaml:"schema_version" json:"schema_version"`
	ID            string         `yaml:"id" json:"id"`
	DisplayName   string         `yaml:"display_name" json:"display_name"`
	RoleType      string         `yaml:"role_type" json:"role_type"`
	Title         string         `yaml:"title" json:"title"`
	Version       string         `yaml:"version" json:"version"`
	Lifecycle     string         `yaml:"lifecycle" json:"lifecycle"`
	Mission       string         `yaml:"mission" json:"mission"`
	Skills        ProfileSkills  `yaml:"skills" json:"skills"`
}

// ProfileSkills declares where the role's skill package lives.
type ProfileSkills struct {
	Directory       string `yaml:"directory" json:"directory"`
	MetaEntrypoint  string `yaml:"meta_entrypoint" json:"meta_entrypoint"`
	EnvExample      string `yaml:"env_example" json:"env_example"`
	Items           []ProfileSkillItem `yaml:"items" json:"items"`
}

// ProfileSkillItem is one declared role-owned skill.
type ProfileSkillItem struct {
	ID         string `yaml:"id" json:"id"`
	Name       string `yaml:"name" json:"name"`
	UseWhen    string `yaml:"use_when" json:"use_when"`
	Entrypoint string `yaml:"entrypoint" json:"entrypoint"`
	Status     string `yaml:"status" json:"status"`
}

// MCPConfig mirrors mcp/mcp.json. The shape mirrors the MCP standard server
// map; Multica validates ${ENV_NAME} references against declared env keys.
type MCPConfig struct {
	MCPServers map[string]any `yaml:"mcpServers" json:"mcpServers"`
}

// EnvDeclaration is one variable recovered from env/.env.example. It carries
// metadata only — never a value.
type EnvDeclaration struct {
	Name        string `json:"name"`
	Required    bool   `json:"required"`    // best-effort: true when referenced by MCP/capability and no default
	Description string `json:"description"` // recovered from leading example comments, "" if unknown
	Configured  bool   `json:"configured"`  // true when env/.env provides the key
	Secret      bool   `json:"secret"`      // heuristic: name contains SECRET/TOKEN/KEY/PASSWORD
}

// EnvFile is the parsed result of reading an env file (example or real).
// Values are present only on the daemon, only for the exact {role}/env/.env
// path, and only for the duration of the explicit apply request.
type EnvFile struct {
	// Declarations is keyed by variable name. For .env.example this carries
	// description/required hints with empty values; for .env it carries the
	// configured boolean.
	Declarations map[string]EnvDeclaration `json:"-"`
	// Values is keyed by variable name. Present only for {role}/env/.env.
	// NEVER serialized into a scan manifest, snapshot, plan, log, or ordinary
	// API response. Use SanitizeEnvForPreview to produce the safe preview form.
	Values map[string]string `json:"-"`
	// Order preserves first-seen variable order for deterministic output.
	Order []string `json:"order"`
}
