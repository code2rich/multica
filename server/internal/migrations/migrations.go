package migrations

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/multica-ai/multica/server/internal/selfexec"
)

const maxSearchDepth = 4

var candidateLeaves = []string{
	"migrations",
	filepath.Join("server", "migrations"),
}

// legacyApplyOrder preserves dependency order for historical migrations whose
// numeric prefixes were shifted in the long-lived AgentWaker fork. Their
// version names are already recorded in deployed databases, so renaming them
// would make the runner treat them as new migrations and reapply destructive
// DDL. Keep the persisted identities and override only their in-memory sort
// positions:
//
//   - resource_labels creates columns/tables required by 167-170.
//   - agent_builder creates columns required by 172_agent_system_identity_index.
//
// Down migrations use the exact reverse of this apply order.
var legacyApplyOrder = map[string]string{
	"173_resource_labels": "166z_resource_labels",
	"174_agent_builder":   "171z_agent_builder",
}

// ResolveDir returns the first migrations directory that exists from the
// current working directory.
func ResolveDir() (string, error) {
	seen := make(map[string]bool)
	for _, root := range searchRoots() {
		base := root
		for range maxSearchDepth + 1 {
			for _, leaf := range candidateLeaves {
				dir := filepath.Clean(filepath.Join(base, leaf))
				if seen[dir] {
					continue
				}
				seen[dir] = true
				info, err := os.Stat(dir)
				if err == nil && info.IsDir() {
					return dir, nil
				}
			}
			base = filepath.Join(base, "..")
		}
	}
	return "", fmt.Errorf("migrations directory not found")
}

func searchRoots() []string {
	roots := []string{"."}
	if exe, err := selfexec.Resolve(); err == nil {
		roots = append(roots, filepath.Dir(exe))
	}
	return roots
}

// Files returns sorted migration files for the given direction ("up" or
// "down").
func Files(direction string) ([]string, error) {
	dir, err := ResolveDir()
	if err != nil {
		return nil, err
	}

	suffix := "." + direction + ".sql"
	files, err := filepath.Glob(filepath.Join(dir, "*"+suffix))
	if err != nil {
		return nil, err
	}

	sortMigrationFiles(files, direction)
	return files, nil
}

func sortMigrationFiles(files []string, direction string) {
	sort.Slice(files, func(i, j int) bool {
		left := migrationApplySortKey(files[i])
		right := migrationApplySortKey(files[j])
		if direction == "down" {
			return left > right
		}
		return left < right
	})
}

func migrationApplySortKey(filename string) string {
	version := ExtractVersion(filename)
	if key, ok := legacyApplyOrder[version]; ok {
		return key
	}
	return version
}

// AllVersions returns every "up" migration version found on disk, in apply
// order. The readiness check verifies that all of them are recorded in
// schema_migrations — checking only the lexically-last version would miss an
// out-of-order migration (one numbered below an already-applied later one),
// letting a server report ready while running against a schema that lacks it.
func AllVersions() ([]string, error) {
	files, err := Files("up")
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no up migrations found")
	}
	versions := make([]string, len(files))
	for i, f := range files {
		versions[i] = ExtractVersion(f)
	}
	return versions, nil
}

// ExtractVersion strips the .up.sql / .down.sql suffix from a migration file.
func ExtractVersion(filename string) string {
	base := filepath.Base(filename)
	base = strings.TrimSuffix(base, ".up.sql")
	base = strings.TrimSuffix(base, ".down.sql")
	return base
}
