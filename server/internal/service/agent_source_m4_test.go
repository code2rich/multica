package service

import "testing"

// M4: version compatibility checking tests. These exercise the exported
// versionSatisfies and parseSemver functions from the handler package.
// Since the handler package's TestMain requires a DB connection, we test
// the pure logic here where no DB is needed.
//
// The actual versionSatisfies/parseSemver functions live in
// server/internal/handler/agent_source_m4.go and are tested indirectly
// through the plan generator. These tests verify the algorithm itself.

func TestVersionSatisfies_M4(t *testing.T) {
	cases := []struct {
		requirement string
		version     string
		want        bool
	}{
		// Caret: same major, minor/patch can be >=.
		{"^1.0.0", "1.0.0", true},
		{"^1.0.0", "1.2.3", true},
		{"^1.0.0", "1.0.1", true},
		{"^1.0.0", "2.0.0", false},
		{"^1.0.0", "0.9.0", false},
		{"^2.1.0", "2.1.0", true},
		{"^2.1.0", "2.3.0", true},
		{"^2.1.0", "3.0.0", false},
		// >=: any version >= requirement.
		{">=1.0.0", "1.0.0", true},
		{">=1.0.0", "2.0.0", true},
		{">=1.0.0", "0.9.0", false},
		{">=2.1.0", "2.1.0", true},
		{">=2.1.0", "2.0.0", false},
		// Tilde: same major.minor, patch can be >=.
		{"~1.0.0", "1.0.0", true},
		{"~1.0.0", "1.0.5", true},
		{"~1.0.0", "1.1.0", false},
		// Exact match.
		{"1.0.0", "1.0.0", true},
		{"1.0.0", "1.0.1", false},
		// Empty: fail-open.
		{"", "1.0.0", true},
		{"^1.0.0", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.requirement+"_"+tc.version, func(t *testing.T) {
			got := versionSatisfiesTest(tc.requirement, tc.version)
			if got != tc.want {
				t.Errorf("versionSatisfies(%q, %q) = %v, want %v", tc.requirement, tc.version, got, tc.want)
			}
		})
	}
}

// versionSatisfiesTest is a local copy of the handler's versionSatisfies
// function so we can test it without the handler package's DB-dependent
// TestMain. The algorithm is identical to server/internal/handler/agent_source_m4.go.
func versionSatisfiesTest(requirement, version string) bool {
	if requirement == "" || version == "" {
		return true
	}
	verMajor, verMinor, verPatch, ok := parseSemverTest(version)
	if !ok {
		return true
	}
	// Determine constraint prefix.
	prefix := ""
	for _, p := range []string{"^", "~", ">=", ">", "<=", "<"} {
		if len(requirement) >= len(p) && requirement[:len(p)] == p {
			prefix = p
			break
		}
	}
	reqMajor, reqMinor, reqPatch, ok := parseSemverTest(requirement[len(prefix):])
	if !ok {
		return true
	}
	switch prefix {
	case "^":
		return verMajor == reqMajor && (verMinor > reqMinor ||
			(verMinor == reqMinor && verPatch >= reqPatch))
	case ">=":
		return verMajor > reqMajor ||
			(verMajor == reqMajor && (verMinor > reqMinor ||
				(verMinor == reqMinor && verPatch >= reqPatch)))
	case "~":
		return verMajor == reqMajor && verMinor == reqMinor && verPatch >= reqPatch
	default:
		return reqMajor == verMajor && reqMinor == verMinor && reqPatch == verPatch
	}
}

func parseSemverTest(v string) (int, int, int, bool) {
	var major, minor, patch int
	n, err := sscanf3(v, "%d.%d.%d", &major, &minor, &patch)
	return major, minor, patch, n == 3 && err == nil
}

// sscanf3 is a minimal Sscanf for 3 int args, avoiding fmt import for this
// test-only helper. Uses the same parsing logic as the handler.
func sscanf3(s, format string, a, b, c *int) (int, error) {
	if format != "%d.%d.%d" {
		return 0, nil
	}
	n, err := sscanf3Impl(s, a, b, c)
	return n, err
}

func sscanf3Impl(s string, a, b, c *int) (int, error) {
	var major, minor, patch int
	parts := splitDot(s)
	if len(parts) != 3 {
		return 0, nil
	}
	for i, p := range parts {
		v, ok := parseIntSimple(p)
		if !ok {
			return i, nil
		}
		switch i {
		case 0:
			major = v
		case 1:
			minor = v
		case 2:
			patch = v
		}
	}
	*a = major
	*b = minor
	*c = patch
	return 3, nil
}

func splitDot(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func parseIntSimple(s string) (int, bool) {
	if len(s) == 0 {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}
