import type { AgentSourceFile } from "@multica/core/types";

/** Resolve a relative source-file link without allowing it to leave the role root. */
export function resolveAgentSourcePath(
  currentPath: string,
  href: string,
): string | null {
  const raw = href.trim();
  if (
    !raw ||
    raw.startsWith("#") ||
    raw.startsWith("/") ||
    /^[a-z][a-z0-9+.-]*:/i.test(raw)
  ) {
    return null;
  }

  const withoutSuffix = raw.split(/[?#]/, 1)[0] ?? "";
  const base = currentPath.includes("/")
    ? currentPath.slice(0, currentPath.lastIndexOf("/") + 1)
    : "";
  const parts: string[] = [];
  for (const part of `${base}${withoutSuffix}`.replaceAll("\\", "/").split("/")) {
    if (!part || part === ".") continue;
    if (part === "..") {
      if (parts.length === 0) return null;
      parts.pop();
      continue;
    }
    parts.push(part);
  }
  return parts.length > 0 ? parts.join("/") : null;
}

export function findAgentSourceFile(
  files: AgentSourceFile[] | undefined,
  currentPath: string,
  href: string,
): { path: string; file?: AgentSourceFile } | null {
  const path = resolveAgentSourcePath(currentPath, href);
  if (!path) return null;
  return { path, file: files?.find((candidate) => candidate.path === path) };
}
