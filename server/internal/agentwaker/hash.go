package agentwaker

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Canonical content hashing for the AgentWaker directory integration.
//
// All hashes are SHA-256 and length-prefixed (the same framing convention as
// server/pkg/skillbundle/hash.go) so concatenated fields cannot collide.
// DirectoryHash and CapabilityContentHash are pure functions of public content
// and therefore unkeyed; they are safe to store in snapshots and share across
// daemon/server.
//
// EnvValueDigest is different: it covers plaintext values and is therefore
// HMAC-keyed by a server-held secret. The digest is non-reversible and used
// only for change detection in previews/plans/diffs. It MUST NOT be derivable
// without the server secret, so the daemon obtains digests by calling
// EnvValueDigest with the same key material the server uses.

// hashPart writes one length-prefixed field to a running hasher.
func hashPart(h interface{ Write([]byte) (int, error) }, value string) {
	_, _ = fmt.Fprintf(h, "%d:%s\n", len(value), value)
}

// SkillBundleHash reuses the established skill-bundle algorithm. The input is
// the same shape the runtime already hashes, so bundles compared across the
// directory sync and the existing skill cache agree byte-for-byte. Callers
// build the skillbundle.Skill value from the parsed role skill.
func SkillBundleHash(source, id, name, description, content string, files []SkillBundleFile) string {
	sorted := append([]SkillBundleFile(nil), files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })

	h := sha256.New()
	hashPart(h, "v1")
	hashPart(h, source)
	hashPart(h, id)
	hashPart(h, name)
	hashPart(h, description)
	hashPart(h, content)
	for _, f := range sorted {
		fileDigest := "sha256:" + sha256Hex([]byte(f.Content))
		hashPart(h, f.Path)
		hashPart(h, fileDigest)
		hashPart(h, f.Content)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// SkillBundleFile is the public input to SkillBundleHash.
type SkillBundleFile struct {
	Path    string
	Content string
}

// CapabilityContentHash produces a deterministic digest for a capability
// package: manifest metadata + entrypoint content + supporting files (sorted).
// This is what the directory snapshot stores so two scans of the same package
// compare equal without re-reading file bodies.
func CapabilityContentHash(manifest CapabilityManifest, entrypointContent string, supporting []SkillBundleFile) string {
	h := sha256.New()
	hashPart(h, "agentwaker-capability-v1")
	hashPart(h, manifest.ID)
	hashPart(h, manifest.Version)
	hashPart(h, manifest.Name)
	hashPart(h, manifest.Description)
	hashPart(h, manifest.Entrypoint)
	hashPart(h, entrypointContent)
	// Profiles and adapters participate in identity: a new/removed profile
	// changes the package even at the same version.
	for _, p := range sortedProfiles(manifest.Profiles) {
		hashPart(h, p.ID)
		hashPart(h, p.Description)
	}
	for _, a := range sortedAdapters(manifest.Adapters) {
		hashPart(h, a.ID)
		hashPart(h, fmt.Sprintf("%t", a.Required))
	}
	for _, f := range sortedFiles(supporting) {
		hashPart(h, f.Path)
		hashPart(h, "sha256:"+sha256Hex([]byte(f.Content)))
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// DailyAutomationContentHash covers every source-authoritative field plus the
// exact prompt bytes. Workspace-owned activation is represented by the fixed
// source contract, not by the mutable database value.
func DailyAutomationContentHash(automation DailyAutomation, promptContent string) string {
	h := sha256.New()
	hashPart(h, "agentwaker-daily-automation-v1")
	hashPart(h, automation.ID)
	hashPart(h, automation.Title)
	hashPart(h, automation.PromptFile)
	hashPart(h, promptContent)
	hashPart(h, automation.Execution.Mode)
	hashPart(h, automation.Execution.IssueTitleTemplate)
	hashPart(h, automation.Schedule.Kind)
	hashPart(h, automation.Schedule.Expression)
	hashPart(h, automation.Schedule.Timezone)
	hashPart(h, fmt.Sprintf("%t", automation.Schedule.InitialEnabled))
	hashPart(h, automation.Schedule.Label)
	hashPart(h, automation.Sync.Content)
	hashPart(h, automation.Sync.Schedule)
	hashPart(h, automation.Sync.Activation)
	hashPart(h, automation.Sync.Missing)
	hashPart(h, automation.Governance.ExternalWrites)
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// DirectoryHash is the canonical digest of a sanitized scan manifest. The
// manifest passed in MUST already be value-free (env redacted); this function
// is pure and unkeyed so the daemon and server independently compute the same
// digest over the same sanitized bytes.
//
// The input is marshaled canonically (sorted object keys) before hashing, so
// map iteration order cannot change the digest.
func DirectoryHash(manifest any) (string, error) {
	canonical, err := canonicalJSON(manifest)
	if err != nil {
		return "", fmt.Errorf("agentwaker: canonicalize manifest: %w", err)
	}
	h := sha256.New()
	hashPart(h, "agentwaker-directory-v1")
	hashPart(h, string(canonical))
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// EnvValueDigest is a non-reversible, HMAC-keyed digest of one env value. The
// key is the server's env-secret key (the same one used for at-rest envelope
// encryption). The daemon computes digests with key material supplied by the
// server for the scan session, so previews carry digests — never values — and
// the server can still detect "this value changed" by recomputing.
//
// The digest includes the variable name so renaming a key while keeping the
// value is still detected as a change.
func EnvValueDigest(key, value string, hmacKey []byte) string {
	mac := hmac.New(sha256.New, hmacKey)
	hashPart(mac, "agentwaker-env-digest-v1")
	hashPart(mac, key)
	hashPart(mac, value)
	return "hmac-sha256:" + hex.EncodeToString(mac.Sum(nil))
}

// --- internal helpers ---

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func sortedProfiles(ps []CapabilityProfile) []CapabilityProfile {
	out := append([]CapabilityProfile(nil), ps...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func sortedAdapters(as []CapabilityAdapter) []CapabilityAdapter {
	out := append([]CapabilityAdapter(nil), as...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func sortedFiles(fs []SkillBundleFile) []SkillBundleFile {
	out := append([]SkillBundleFile(nil), fs...)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// canonicalJSON marshals a value with sorted map keys and no incidental
// whitespace, so two structurally-equal values hash identically regardless of
// Go map iteration order or struct field source order.
func canonicalJSON(v any) ([]byte, error) {
	// Two-pass: marshal then re-marshal via the token decoder with sorted keys.
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var anyVal any
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&anyVal); err != nil {
		return nil, err
	}
	var buf strings.Builder
	if err := writeCanonicalJSON(&buf, anyVal); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}

// writeCanonicalJSON emits canonical JSON: object keys sorted ascending,
// arrays in order, numbers via their original text, compact spacing.
func writeCanonicalJSON(buf *strings.Builder, v any) error {
	switch t := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case json.Number:
		buf.WriteString(string(t))
	case string:
		b, err := json.Marshal(t)
		if err != nil {
			return err
		}
		buf.Write(b)
	case []any:
		buf.WriteByte('[')
		for i, item := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonicalJSON(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			kb, _ := json.Marshal(k)
			buf.Write(kb)
			buf.WriteByte(':')
			if err := writeCanonicalJSON(buf, t[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	default:
		return fmt.Errorf("agentwaker: unsupported canonical JSON type %T", v)
	}
	return nil
}
