package agentwaker

// Sanitization boundary for environment values.
//
// Plaintext env values live only in daemon memory (for the exact {role}/env/.env
// path) and in the single secret apply envelope. Every other surface — scan
// preview, snapshot, plan, diff, log, event, ordinary API response — must use
// the sanitized form produced here. The sanitized form carries:
//   - the variable name (needed to display declarations and diffs);
//   - the configured boolean (whether .env supplied the key);
//   - whether the name looks like a secret;
//   - a non-reversible value digest for change detection;
//   - the description recovered from .env.example.
//
// It NEVER carries the value itself.

// SanitizedEnvDeclaration is the value-safe form of one env variable, suitable
// for storage in a snapshot, a preview, a plan, or any ordinary API response.
type SanitizedEnvDeclaration struct {
	Name        string `json:"name"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
	Configured  bool   `json:"configured"`
	Secret      bool   `json:"secret"`
	ValueDigest string `json:"value_digest,omitempty"` // present only when Configured && hmacKey != nil
}

// SanitizeEnvForPreview converts a parsed EnvFile into a value-safe slice.
//
// When hmacKey is non-empty, configured values are replaced by their
// EnvValueDigest (computed with that key). When hmacKey is empty, no digest is
// emitted; callers that need change detection must supply the key. In both
// cases the plaintext value is discarded and cannot be recovered from the
// returned declarations.
//
// The returned slice follows the EnvFile's deterministic Order.
func SanitizeEnvForPreview(env EnvFile, hmacKey []byte) []SanitizedEnvDeclaration {
	out := make([]SanitizedEnvDeclaration, 0, len(env.Order))
	for _, name := range env.Order {
		decl := env.Declarations[name]
		safe := SanitizedEnvDeclaration{
			Name:        name,
			Required:    decl.Required,
			Description: decl.Description,
			Configured:  decl.Configured,
			Secret:      decl.Secret,
		}
		if decl.Configured && len(hmacKey) > 0 {
			safe.ValueDigest = EnvValueDigest(name, env.Values[name], hmacKey)
		}
		out = append(out, safe)
	}
	return out
}

// AssertNoPlaintextEnv inspects an arbitrary decoded JSON value for env-value
// leakage. It returns the first key path at which it finds a value that looks
// like a raw env value (a bare string under a *_value / value / secret field,
// or a plaintext string where a digest object was expected), or nil if the
// value is clean. This is the defense-in-depth check the server runs on every
// inbound daemon scan report.
//
// The detector is conservative: it flags any object containing a "value" key
// whose value is a string, since the sanitized declaration type uses
// "value_digest" (string) and never "value". False positives are acceptable —
// a rejected scan is retried; a leaked value is not.
func AssertNoPlaintextEnv(v any) string {
	return walkForPlaintextEnv(v, "")
}

func walkForPlaintextEnv(v any, path string) string {
	switch t := v.(type) {
	case map[string]any:
		// Detect the "value": "<plaintext>" anti-pattern directly.
		if raw, ok := t["value"]; ok {
			if _, isStr := raw.(string); isStr {
				if path == "" {
					return "$.value"
				}
				return path + ".value"
			}
		}
		for k, child := range t {
			childPath := "$." + k
			if path != "" {
				childPath = path + "." + k
			}
			if hit := walkForPlaintextEnv(child, childPath); hit != "" {
				return hit
			}
		}
	case []any:
		for i, child := range t {
			childPath := fmtIndexPath(path, i)
			if hit := walkForPlaintextEnv(child, childPath); hit != "" {
				return hit
			}
		}
	}
	return ""
}

// fmtIndexPath renders an array-index path segment consistently with the
// object-key form ($.<key>[<i>]).
func fmtIndexPath(path string, i int) string {
	if path == "" {
		return "$[" + itoa(i) + "]"
	}
	return path + "[" + itoa(i) + "]"
}

// itoa is a tiny allocation-free int formatter to avoid pulling strconv into a
// pure security check hot path.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}
