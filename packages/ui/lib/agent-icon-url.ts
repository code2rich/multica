/**
 * Pure-string helpers for the built-in agent icon library. No React, no API —
 * safe to call from anywhere. Lives in `packages/ui` (not `packages/core`)
 * because `ActorAvatarBase` needs these at render time and `packages/ui` may
 * not import `@multica/core`.
 *
 * Storage contract: a built-in icon is encoded on `agent.avatar_url` as
 * `"icon:<key>"` (e.g. `"icon:robot"`). `resolvePublicFileUrl` only rewrites
 * strings that start with `/`, so the `icon:` scheme passes through every
 * existing resolver unchanged and reaches the renderer as-is.
 */

import {
  AGENT_ICON_PREFIX,
  AGENT_ICON_KEYS,
  type AgentIconKey,
} from "../components/common/agent-icons";

/** Type guard: does this avatar_url reference a built-in icon? */
export function isAgentIconUrl(
  url: string | null | undefined,
): url is string {
  return typeof url === "string" && url.startsWith(AGENT_ICON_PREFIX);
}

/**
 * Extracts the icon key from an `icon:<key>` avatar_url, or `null` if the url
 * is not an icon reference or the key is not in the registry. Note:
 * {@link isAgentIconUrl} matches the prefix only — this is the validity check.
 */
export function agentIconKeyFromUrl(
  url: string | null | undefined,
): AgentIconKey | null {
  if (!isAgentIconUrl(url)) return null;
  const key = url.slice(AGENT_ICON_PREFIX.length);
  return (AGENT_ICON_KEYS as readonly string[]).includes(key)
    ? (key as AgentIconKey)
    : null;
}

/** Encodes a known icon key as an `avatar_url` value. */
export function buildAgentIconUrl(key: AgentIconKey): string {
  return `${AGENT_ICON_PREFIX}${key}`;
}

/**
 * Deterministic name → icon mapping (djb2 hash modulo the icon count). The
 * same name always yields the same icon across sessions, reloads, and
 * clients, so a freshly-created agent with no avatar gets a stable identity
 * and pre-existing agents without an avatar get a consistent fallback without
 * any backfill. Append-only key order in `AGENT_ICON_KEYS` keeps this stable.
 */
export function defaultAgentIconKey(name: string): AgentIconKey {
  const trimmed = name.trim();
  let hash = 5381;
  for (let i = 0; i < trimmed.length; i++) {
    hash = (((hash << 5) + hash) ^ trimmed.charCodeAt(i)) >>> 0;
  }
  return AGENT_ICON_KEYS[hash % AGENT_ICON_KEYS.length]!;
}
