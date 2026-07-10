import { parse as parseYaml } from "yaml";

/**
 * Parses a YAML document string into a typed value. Returns `null` on parse
 * failure or when the document is empty / not a mapping, so callers can treat
 * a missing-but-present file the same as an absent one.
 *
 * Re-exported from `@multica/core/skills` so views can parse standalone YAML
 * files (e.g. an agentwaker `PROFILE.yaml`) without taking a direct dependency
 * on the `yaml` package — `@multica/core` is already a workspace dependency.
 */
export function parseYamlDocument<T = unknown>(raw: string): T | null {
  let parsed: unknown;
  try {
    parsed = parseYaml(raw);
  } catch {
    return null;
  }
  if (parsed == null) return null;
  return parsed as T;
}
