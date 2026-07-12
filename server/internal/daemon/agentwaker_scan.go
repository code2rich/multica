package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/multica-ai/multica/server/internal/agentwaker"
)

// ScannerVersion is the daemon-side scanner version reported with each result.
// Bump when the scan algorithm or hashing changes so the server can reconcile.
const ScannerVersion = "1"

// agentWakerEnvDigestKey returns the HMAC key the scanner uses to compute env
// value digests. When no key is configured, digests are omitted and previews
// carry only key names + configured booleans (still value-safe).
func (d *Daemon) agentWakerEnvDigestKey() []byte {
	return d.cfg.AgentWakerEnvDigestKey
}

// ScanResult is the sanitized output of one directory scan, ready to be reported
// to the server. It never carries plaintext env values — only key names,
// configured booleans, and value digests. DirectoryHash is the canonical digest
// of the sanitized manifest.
type ScanResult struct {
	DirectoryHash  string                     `json:"directory_hash"`
	SchemaVersions map[string]string          `json:"schema_versions"`
	Manifest       map[string]any             `json:"manifest"`
	Diagnostics    []agentwakerScanDiagnostic `json:"diagnostics"`
	ScannerVersion string                     `json:"scanner_version"`
}

// agentwakerScanDiagnostic is the daemon-side diagnostic shape; converted to
// handler.ScanDiagnostic by the report path.
type agentwakerScanDiagnostic struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
}

// skipDirNames are directory entries the scanner ignores entirely. They never
// participate in a role/capability contract and may be huge or transient.
var skipDirNames = map[string]bool{
	".git": true, "node_modules": true, ".idea": true, "workdir": true,
	"__pycache__": true, ".DS_Store": true,
}

// binaryExtensions are file extensions the text-only scan excludes from skill
// bundles. Declared binary assets will enter the content-addressed artifact
// pipeline in M3; in M1 they are reported as skipped so nothing is lost silently.
var binaryExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
	".pdf": true, ".zip": true, ".tar": true, ".gz": true, ".ico": true,
	".mp4": true, ".mov": true, ".mp3": true, ".wav": true, ".exe": true,
	".dll": true, ".so": true, ".dylib": true, ".pyc": true, ".class": true,
	".sqlite": true, ".db": true,
}

const (
	maxSkillFileSize    = 1 << 20 // 1 MiB per file (matches directory-import.ts)
	maxSkillTotalSize   = 8 << 20 // 8 MiB aggregate
	maxSkillFileCount   = 128
	maxInstructionsSize = 1 << 20 // 1 MiB for agent-detail.en.md
)

// ScanDirectory performs a read-only scan of one configured AgentWaker root.
// It reuses the daemon path validators (canonical path, ownership, symlink, and
// forbidden-root checks) so the configured directory obeys the same security
// rules as a runtime-local project directory. The scan NEVER executes scripts,
// NEVER writes to the source tree, and NEVER returns plaintext env values.
func ScanDirectory(ctx context.Context, absPath string, envDigestKey []byte) (*ScanResult, error) {
	// Reuse the established daemon path-validation pipeline.
	if err := validateLocalPath(absPath); err != nil {
		return nil, err
	}

	// Repository discovery: the three required signals.
	registryPath := filepath.Join(absPath, "capabilities", "registry.yaml")
	profileSchemaPath := filepath.Join(absPath, "schemas", "profile-v2.1.schema.json")
	if _, err := os.Stat(registryPath); err != nil {
		return nil, fmt.Errorf("agentwaker: capabilities/registry.yaml not found at %q", absPath)
	}
	if _, err := os.Stat(profileSchemaPath); err != nil {
		return nil, fmt.Errorf("agentwaker: schemas/profile-v2.1.schema.json not found at %q", absPath)
	}

	diags := []agentwakerScanDiagnostic{}
	addDiag := func(sev, code, msg, path string) {
		diags = append(diags, agentwakerScanDiagnostic{Severity: sev, Code: code, Message: msg, Path: path})
	}

	// --- registry + capabilities ---
	registryBytes, err := os.ReadFile(registryPath)
	if err != nil {
		return nil, fmt.Errorf("read registry.yaml: %w", err)
	}
	registry, err := agentwaker.ParseRegistry(registryBytes)
	if err != nil {
		return nil, fmt.Errorf("parse registry.yaml: %w", err)
	}

	capabilities := []map[string]any{}
	capabilitiesRoot := filepath.Join(absPath, "capabilities")
	for _, entry := range registry.Capabilities {
		// The registry's manifest field is relative to capabilities/ (e.g.
		// "information-collection/CAPABILITY.yaml").
		capPath := filepath.Join(capabilitiesRoot, filepath.Clean(entry.Manifest))
		cap, capDiags, err := scanCapability(absPath, capPath, entry)
		if err != nil {
			addDiag("error", "capability_parse", err.Error(), rel(absPath, capPath))
			continue
		}
		diags = append(diags, capDiags...)
		capabilities = append(capabilities, cap)
	}

	// --- roles: every top-level dir with agent-soul/PROFILE.yaml ---
	roles := []map[string]any{}
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, fmt.Errorf("read root: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if !entry.IsDir() || skipDirNames[entry.Name()] {
			continue
		}
		roleDir := filepath.Join(absPath, entry.Name())
		profilePath := filepath.Join(roleDir, "agent-soul", "PROFILE.yaml")
		if _, err := os.Stat(profilePath); err != nil {
			continue // not a role directory (e.g. visual-research output)
		}
		role, roleDiags, err := scanRole(absPath, roleDir, envDigestKey)
		if err != nil {
			addDiag("error", "role_parse", err.Error(), entry.Name())
			continue
		}
		diags = append(diags, roleDiags...)
		roles = append(roles, role)
	}

	// --- assemble sanitized manifest + canonical directory hash ---
	manifest := map[string]any{
		"capabilities": capabilities,
		"roles":        roles,
	}
	dirHash, err := agentwaker.DirectoryHash(manifest)
	if err != nil {
		return nil, fmt.Errorf("compute directory hash: %w", err)
	}

	return &ScanResult{
		DirectoryHash: dirHash,
		SchemaVersions: map[string]string{
			"profile":    agentwaker.ProfileSchemaVersion,
			"capability": agentwaker.CapabilitySchemaVersion,
			"registry":   agentwaker.RegistrySchemaVersion,
		},
		Manifest:       manifest,
		Diagnostics:    diags,
		ScannerVersion: ScannerVersion,
	}, nil
}

// scanCapability reads one capabilities/{id}/CAPABILITY.yaml + its entrypoint +
// supporting text files and returns the sanitized manifest entry. The content
// hash makes the package reproducible without embedding file bodies.
func scanCapability(root, capManifestPath string, regEntry agentwaker.RegistryCapability) (map[string]any, []agentwakerScanDiagnostic, error) {
	diags := []agentwakerScanDiagnostic{}
	manifestBytes, err := os.ReadFile(capManifestPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read CAPABILITY.yaml: %w", err)
	}
	manifest, err := agentwaker.ParseCapabilityManifest(manifestBytes)
	if err != nil {
		return nil, nil, err
	}
	capDir := filepath.Dir(capManifestPath)

	// Entrypoint content (e.g. SKILL.md).
	entrypointPath := filepath.Join(capDir, filepath.Clean(manifest.Entrypoint))
	entrypointContent, err := readTextFile(entrypointPath, maxSkillFileSize)
	if err != nil {
		diags = append(diags, agentwakerScanDiagnostic{Severity: "warning", Code: "entrypoint_missing", Message: err.Error(), Path: rel(root, entrypointPath)})
		entrypointContent = ""
	}

	// Supporting text files (schemas, docs) — exclude the entrypoint itself and
	// binary assets, which are reported as skipped.
	supporting := []agentwaker.SkillBundleFile{}
	skipped := []string{}
	if err := filepath.WalkDir(capDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if path == entrypointPath || filepath.Clean(path) == filepath.Clean(capManifestPath) {
			return nil
		}
		relPath := rel(capDir, path)
		if binaryExtensions[strings.ToLower(filepath.Ext(path))] {
			skipped = append(skipped, relPath)
			return nil
		}
		content, rerr := readTextFile(path, maxSkillFileSize)
		if rerr != nil {
			skipped = append(skipped, relPath)
			return nil
		}
		supporting = append(supporting, agentwaker.SkillBundleFile{Path: relPath, Content: content})
		return nil
	}); err != nil {
		return nil, diags, fmt.Errorf("walk capability dir: %w", err)
	}

	contentHash := agentwaker.CapabilityContentHash(manifest, entrypointContent, supporting)
	// The capability content (entrypoint body + supporting text files) is public
	// text — it is NOT env/secret data — so it travels in the snapshot so the
	// M3 runtime materialization path can write it into execution sandboxes
	// without a second daemon round-trip. The content-addressed hash above
	// already covers these bodies; including them is content duplication for
	// delivery, not a new identity.
	supportingForManifest := make([]map[string]any, 0, len(supporting))
	for _, f := range supporting {
		supportingForManifest = append(supportingForManifest, map[string]any{
			"path":    f.Path,
			"content": f.Content,
		})
	}
	entry := map[string]any{
		"id":            manifest.ID,
		"name":          manifest.Name,
		"version":       manifest.Version,
		"description":   manifest.Description,
		"entrypoint":    manifest.Entrypoint,
		"content_hash":  contentHash,
		"profile_count": len(manifest.Profiles),
		"adapter_count": len(manifest.Adapters),
		"permissions": map[string]any{
			"default_mode":             manifest.Permissions.DefaultMode,
			"supports_account_actions": manifest.Permissions.SupportsAccountActions,
		},
		// Runtime materialization content (M3). The entrypoint body becomes the
		// bundle Content; supporting files become bundle Files[]. Binary assets
		// are reported as skipped diagnostics above and enter the content-
		// addressed artifact pipeline in a later milestone.
		"entrypoint_content": entrypointContent,
		"supporting_files":   supportingForManifest,
	}
	for _, s := range skipped {
		diags = append(diags, agentwakerScanDiagnostic{Severity: "warning", Code: "binary_skipped", Message: "binary or oversized file skipped", Path: rel(root, filepath.Join(capDir, s))})
	}
	return entry, diags, nil
}

// scanRole reads one role directory: PROFILE.yaml, agent-detail.en.md,
// agent-persona.html, the skill package, capabilities.yaml, env declarations +
// values (sanitized), and mcp/mcp.json. Returns the sanitized manifest entry.
func scanRole(root, roleDir string, envDigestKey []byte) (map[string]any, []agentwakerScanDiagnostic, error) {
	diags := []agentwakerScanDiagnostic{}
	roleName := filepath.Base(roleDir)

	profileBytes, err := os.ReadFile(filepath.Join(roleDir, "agent-soul", "PROFILE.yaml"))
	if err != nil {
		return nil, nil, fmt.Errorf("read PROFILE.yaml: %w", err)
	}
	profile, err := agentwaker.ParseProfile(profileBytes)
	if err != nil {
		return nil, nil, err
	}

	// Instructions (agent-detail.en.md).
	instructionsHash := ""
	instructionsContent := ""
	if content, rerr := readTextFile(filepath.Join(roleDir, "agent-detail.en.md"), maxInstructionsSize); rerr == nil {
		instructionsContent = content
		instructionsHash = agentwaker.SkillBundleHash("agentwaker", profile.ID, "instructions", "", content, nil)
	} else {
		diags = append(diags, agentwakerScanDiagnostic{Severity: "warning", Code: "instructions_missing", Message: "agent-detail.en.md not found", Path: roleName})
	}

	// Persona HTML (agent-persona.html).
	personaHash := ""
	personaContent := ""
	if content, rerr := readTextFile(filepath.Join(roleDir, "agent-persona.html"), maxInstructionsSize); rerr == nil {
		personaContent = content
		personaHash = agentwaker.SkillBundleHash("agentwaker", profile.ID, "persona", "", content, nil)
	} else {
		diags = append(diags, agentwakerScanDiagnostic{Severity: "warning", Code: "persona_missing", Message: "agent-persona.html not found", Path: roleName})
	}

	// Skills package.
	skillsDir := filepath.Join(roleDir, strings.TrimSuffix(profile.Skills.Directory, "/"))
	skills, skillDiags, err := scanRoleSkills(root, roleName, skillsDir, profile)
	if err != nil {
		diags = append(diags, agentwakerScanDiagnostic{Severity: "error", Code: "skills_scan", Message: err.Error(), Path: roleName})
	} else {
		diags = append(diags, skillDiags...)
	}

	// Capabilities dependency manifest (role → capability bindings).
	var bindingsSummary []map[string]any
	if rcBytes, rerr := os.ReadFile(filepath.Join(roleDir, "capabilities.yaml")); rerr == nil {
		rc, perr := agentwaker.ParseRoleCapabilities(rcBytes)
		if perr != nil {
			diags = append(diags, agentwakerScanDiagnostic{Severity: "error", Code: "capabilities_yaml", Message: perr.Error(), Path: filepath.Join(roleName, "capabilities.yaml")})
		} else {
			for _, b := range rc.Capabilities {
				uses := make([]map[string]any, 0, len(b.UsedBy))
				for _, u := range b.UsedBy {
					uses = append(uses, map[string]any{"skill": u.Skill, "profile": u.Profile})
				}
				bindingsSummary = append(bindingsSummary, map[string]any{
					"id":       b.ID,
					"version":  b.Version,
					"required": b.Required,
					"used_by":  uses,
					"mode":     b.Permissions.Mode,
					"fallback": b.Fallback.Behavior,
				})
			}
		}
	}

	// Environment: read the EXACT {role}/env/.env for values and env/.env.example
	// for declarations, then sanitize. Unrelated .env-style files elsewhere are
	// never read. This is the centralized-configuration source of truth.
	envDecls, envDiags := scanRoleEnv(root, roleDir, envDigestKey)
	diags = append(diags, envDiags...)

	// MCP: parse mcp/mcp.json and validate ${ENV} references against declared keys.
	mcpSummary, mcpDiags := scanRoleMCP(roleDir, envDecls)
	diags = append(diags, mcpDiags...)

	entry := map[string]any{
		"id":                   profile.ID,
		"role_dir":             roleName,
		"display_name":         profile.DisplayName,
		"title":                profile.Title,
		"version":              profile.Version,
		"lifecycle":            profile.Lifecycle,
		"mission":              profile.Mission,
		"instructions_content": instructionsContent,
		"instructions_hash":    instructionsHash,
		"persona_content":      personaContent,
		"persona_hash":         personaHash,
		"skills":               skills,
		"capability_bindings":  bindingsSummary,
		"env":                  envDecls,
		"mcp":                  mcpSummary,
	}
	return entry, diags, nil
}

// scanRoleSkills walks the declared skills directory and hashes each specialist
// skill plus the meta entrypoint. Returns sanitized skill descriptors.
func scanRoleSkills(root, roleName, skillsDir string, profile agentwaker.ProfileV2) ([]map[string]any, []agentwakerScanDiagnostic, error) {
	diags := []agentwakerScanDiagnostic{}
	out := []map[string]any{}

	if _, err := os.Stat(skillsDir); err != nil {
		return out, diags, nil // empty skills is valid
	}

	// Specialist skill dirs: each immediate subdir with its own SKILL.md.
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return out, diags, fmt.Errorf("read skills dir: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	specialistDirs := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if info, statErr := os.Stat(filepath.Join(skillsDir, entry.Name(), "SKILL.md")); statErr == nil && !info.IsDir() {
			specialistDirs[entry.Name()] = true
		}
	}

	// Meta entrypoint plus shared role-level support files. Specialist skill
	// subtrees are excluded because each is materialized as its own bundle.
	metaPath := filepath.Join(skillsDir, "SKILL.md")
	if content, rerr := readTextFile(metaPath, maxSkillFileSize); rerr == nil {
		files, skipped, collectErr := collectSkillSupportingFiles(skillsDir, metaPath, specialistDirs)
		if collectErr != nil {
			return out, diags, fmt.Errorf("walk meta skill dir: %w", collectErr)
		}
		hash := agentwaker.SkillBundleHash("agentwaker", profile.ID+":meta", "meta", "", content, files)
		out = append(out, skillManifestEntry(profile.ID+":meta", profile.DisplayName+" (meta)", true, content, hash, files))
		for _, s := range skipped {
			diags = append(diags, agentwakerScanDiagnostic{Severity: "warning", Code: "file_skipped", Message: "supporting file skipped: " + s, Path: rel(root, skillsDir)})
		}
	}

	for _, entry := range entries {
		if !entry.IsDir() || !specialistDirs[entry.Name()] {
			continue
		}
		skillDir := filepath.Join(skillsDir, entry.Name())
		skillEntry, skillDiags, err := scanSpecialistSkill(root, roleName, skillDir)
		if err != nil {
			diags = append(diags, agentwakerScanDiagnostic{Severity: "warning", Code: "skill_scan", Message: err.Error(), Path: rel(root, skillDir)})
			continue
		}
		diags = append(diags, skillDiags...)
		if skillEntry != nil {
			out = append(out, skillEntry)
		}
	}
	return out, diags, nil
}

// scanSpecialistSkill reads one {skill-id}/ directory: SKILL.md content +
// supporting text files, producing a content hash. Binary/oversized files are
// reported as skipped.
func scanSpecialistSkill(root, roleName, skillDir string) (map[string]any, []agentwakerScanDiagnostic, error) {
	diags := []agentwakerScanDiagnostic{}
	skillID := filepath.Base(skillDir)
	contentPath := filepath.Join(skillDir, "SKILL.md")
	content, err := readTextFile(contentPath, maxSkillFileSize)
	if err != nil {
		return nil, diags, fmt.Errorf("read SKILL.md: %w", err)
	}

	files, skipped, err := collectSkillSupportingFiles(skillDir, contentPath, nil)
	if err != nil {
		return nil, diags, fmt.Errorf("walk skill dir: %w", err)
	}

	hash := agentwaker.SkillBundleHash("agentwaker", skillID, skillID, "", content, files)
	for _, s := range skipped {
		diags = append(diags, agentwakerScanDiagnostic{Severity: "warning", Code: "file_skipped", Message: "supporting file skipped: " + s, Path: rel(root, skillDir)})
	}
	return skillManifestEntry(skillID, skillID, false, content, hash, files), diags, nil
}

func collectSkillSupportingFiles(skillDir, contentPath string, skipRootDirs map[string]bool) ([]agentwaker.SkillBundleFile, []string, error) {
	files := []agentwaker.SkillBundleFile{}
	skipped := []string{}
	totalSize := 0
	fileCount := 0
	err := filepath.WalkDir(skillDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if path != skillDir && filepath.Dir(path) == skillDir && skipRootDirs[d.Name()] {
				return filepath.SkipDir
			}
			if path != skillDir && skipDirNames[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Clean(path) == filepath.Clean(contentPath) {
			return nil
		}
		if fileCount >= maxSkillFileCount {
			skipped = append(skipped, rel(skillDir, path)+" (file count limit)")
			return nil
		}
		if binaryExtensions[strings.ToLower(filepath.Ext(path))] {
			skipped = append(skipped, rel(skillDir, path)+" (binary)")
			return nil
		}
		body, rerr := readTextFile(path, maxSkillFileSize)
		if rerr != nil {
			skipped = append(skipped, rel(skillDir, path)+" (unreadable)")
			return nil
		}
		if totalSize+len(body) > maxSkillTotalSize {
			skipped = append(skipped, rel(skillDir, path)+" (total size limit)")
			return nil
		}
		totalSize += len(body)
		fileCount++
		files = append(files, agentwaker.SkillBundleFile{Path: rel(skillDir, path), Content: body})
		return nil
	})
	return files, skipped, err
}

func skillManifestEntry(id, name string, isMeta bool, content, hash string, files []agentwaker.SkillBundleFile) map[string]any {
	supporting := make([]map[string]any, 0, len(files))
	for _, file := range files {
		supporting = append(supporting, map[string]any{"path": file.Path, "content": file.Content})
	}
	return map[string]any{
		"id":                 id,
		"name":               name,
		"is_meta":            isMeta,
		"entrypoint":         "SKILL.md",
		"entrypoint_content": content,
		"supporting_files":   supporting,
		"content_hash":       hash,
		"file_count":         len(files),
	}
}

// scanRoleEnv reads the EXACT env/.env.example for declarations and env/.env for
// values, merges them, sanitizes to digests+metadata, and returns the value-safe
// declarations. The canonical redaction boundary: plaintext never leaves here.
func scanRoleEnv(root, roleDir string, envDigestKey []byte) ([]map[string]any, []agentwakerScanDiagnostic) {
	diags := []agentwakerScanDiagnostic{}
	examplePath := filepath.Join(roleDir, "env", ".env.example")
	realPath := filepath.Join(roleDir, "env", ".env")

	var example, real agentwaker.EnvFile
	if b, err := os.ReadFile(examplePath); err == nil {
		example, _ = agentwaker.ParseEnvFile(b)
	} else {
		diags = append(diags, agentwakerScanDiagnostic{Severity: "warning", Code: "env_example_missing", Message: "env/.env.example not found", Path: filepath.Join(filepath.Base(roleDir), "env", ".env.example")})
	}
	if b, err := os.ReadFile(realPath); err == nil {
		real, _ = agentwaker.ParseEnvFile(b)
	} else {
		// .env is optional (role may be unconfigured); values are absent.
		diags = append(diags, agentwakerScanDiagnostic{Severity: "info", Code: "env_not_configured", Message: "env/.env not found; values not configured", Path: filepath.Join(filepath.Base(roleDir), "env", ".env")})
	}

	merged := agentwaker.MergeEnvDeclarations(example, real)
	safe := agentwaker.SanitizeEnvForPreview(merged, envDigestKey)

	out := make([]map[string]any, 0, len(safe))
	for _, d := range safe {
		out = append(out, map[string]any{
			"name":         d.Name,
			"required":     d.Required,
			"description":  d.Description,
			"configured":   d.Configured,
			"secret":       d.Secret,
			"value_digest": d.ValueDigest,
		})
	}
	return out, diags
}

// scanRoleMCP parses mcp/mcp.json and validates every ${ENV} reference against
// the declared env keys. Missing references are configuration blockers.
func scanRoleMCP(roleDir string, envDecls []map[string]any) (map[string]any, []agentwakerScanDiagnostic) {
	diags := []agentwakerScanDiagnostic{}
	mcpPath := filepath.Join(roleDir, "mcp", "mcp.json")
	summary := map[string]any{"has_servers": false, "server_count": 0}
	b, err := os.ReadFile(mcpPath)
	if err != nil {
		return summary, diags // mcp.json optional
	}
	cfg, err := agentwaker.ParseMCPConfig(b)
	if err != nil {
		diags = append(diags, agentwakerScanDiagnostic{Severity: "error", Code: "mcp_parse", Message: err.Error(), Path: filepath.Join(filepath.Base(roleDir), "mcp", "mcp.json")})
		return summary, diags
	}
	if !cfg.HasServers() {
		return summary, diags
	}

	// Build declared env key set.
	declared := make(map[string]bool, len(envDecls))
	for _, d := range envDecls {
		if name, ok := d["name"].(string); ok {
			declared[name] = true
		}
	}

	// Walk the JSON to find ${NAME} references; each must be a declared key.
	serverCount := 0
	var unresolved []string
	raw := map[string]any(cfg.MCPServers)
	for _, def := range raw {
		serverCount++
		walkEnvRefs(def, func(ref string) {
			if !declared[ref] {
				unresolved = append(unresolved, ref)
			}
		})
	}
	summary["has_servers"] = true
	summary["server_count"] = serverCount
	if len(unresolved) > 0 {
		sort.Strings(unresolved)
		uniq := uniqueStrings(unresolved)
		summary["unresolved_env"] = uniq
		diags = append(diags, agentwakerScanDiagnostic{Severity: "error", Code: "mcp_unresolved_env", Message: "MCP references undeclared env keys: " + strings.Join(uniq, ", "), Path: filepath.Join(filepath.Base(roleDir), "mcp", "mcp.json")})
	}
	return summary, diags
}

// walkEnvRefs calls fn for every ${NAME} token found in any string value within
// the MCP server definition tree.
func walkEnvRefs(v any, fn func(ref string)) {
	switch t := v.(type) {
	case string:
		for _, m := range envRefRE.FindAllStringSubmatch(t, -1) {
			fn(m[1])
		}
	case map[string]any:
		for _, child := range t {
			walkEnvRefs(child, fn)
		}
	case []any:
		for _, child := range t {
			walkEnvRefs(child, fn)
		}
	}
}

var envRefRE = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// --- small helpers ---

func readTextFile(path string, max int) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.Size() > int64(max) {
		return "", fmt.Errorf("file exceeds %d bytes: %s", max, filepath.Base(path))
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// rel returns a root-relative path for diagnostics, falling back to the full
// path when the file is not under root.
func rel(root, path string) string {
	if r, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(r, "..") {
		return r
	}
	return path
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// EncodeDiagnostics converts daemon-side diagnostics to the JSON-decodable shape
// the server report handler expects.
func EncodeDiagnostics(in []agentwakerScanDiagnostic) []byte {
	if len(in) == 0 {
		return []byte("[]")
	}
	b, _ := json.Marshal(in)
	return b
}
