/**
 * Built-in agent avatar icon library.
 *
 * Each entry is a self-contained colorful SVG (a colored disc + a light glyph)
 * so an agent gets a stable, visually distinct identity without uploading a
 * photo. Icons are keyed by a stable slug and referenced from an agent's
 * `avatar_url` as `icon:<key>` (see `@multica/ui/lib/agent-icon-url`).
 *
 * The disc colors are intentionally hardcoded per-icon — like the label
 * palette in `packages/views/issues/components/pickers/label-picker.tsx`, the
 * whole point of this library is per-agent color identity, not theme
 * adaptation. Glyphs are drawn as primitives (no external assets) so the set
 * stays tree-shakeable and SSR-safe (no hooks, no window access).
 *
 * Adding an icon: append an entry to {@link AGENT_ICONS} AND its key to
 * {@link AGENT_ICON_KEYS}. The order of `AGENT_ICON_KEYS` is the contract for
 * `defaultAgentIconKey` (name → key hash), so do not reorder existing keys —
 * only append.
 */

import type { FC, ReactNode } from "react";

/** URL scheme prefix marking an `avatar_url` as a built-in icon reference. */
export const AGENT_ICON_PREFIX = "icon:";

/**
 * Stable, ordered icon slugs. Order is load-bearing — `defaultAgentIconKey`
 * hashes a name to an index, so reordering shifts every existing agent's
 * derived icon. Append-only.
 */
export const AGENT_ICON_KEYS = [
  "robot",
  "nova",
  "spark",
  "orbit",
  "prism",
  "echo",
  "pixel",
  "vector",
  "node",
  "glyph",
  "wave",
  "flux",
  "cipher",
  "atlas",
  "helix",
  "pulse",
] as const;

export type AgentIconKey = (typeof AGENT_ICON_KEYS)[number];

export interface AgentIconProps {
  /** Pixel size of the rendered square. The SVG fills it edge-to-edge. */
  size?: number;
  className?: string;
  /** When set, the SVG exposes `role="img"` + `aria-label`; else `aria-hidden`. */
  title?: string;
}

export interface AgentIconEntry {
  key: AgentIconKey;
  /** English display label for the picker. Proper-noun, not translated. */
  label: string;
  Node: FC<AgentIconProps>;
}

/**
 * Factory: wraps a colored disc + a white glyph into a sized SVG component.
 * Glyph children render inside a `<g>` with white stroke defaults; filled
 * shapes set `fill="#fff" stroke="none"` per-element to override.
 */
function makeIcon(bg: string, glyph: ReactNode): FC<AgentIconProps> {
  function AgentIconSvg({ size = 16, className, title }: AgentIconProps) {
    return (
      <svg
        width={size}
        height={size}
        viewBox="0 0 32 32"
        className={className}
        role={title ? "img" : undefined}
        aria-label={title}
        aria-hidden={title ? undefined : true}
      >
        <circle cx="16" cy="16" r="16" fill={bg} />
        <g
          fill="none"
          stroke="#ffffff"
          strokeWidth={2}
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          {glyph}
        </g>
      </svg>
    );
  }
  return AgentIconSvg;
}

export const AGENT_ICONS: AgentIconEntry[] = [
  {
    key: "robot",
    label: "Robot",
    Node: makeIcon("#64748b", (
      <>
        <line x1="16" y1="3" x2="16" y2="8" />
        <circle cx="16" cy="3" r="1.3" fill="#ffffff" stroke="none" />
        <rect x="8" y="8" width="16" height="15" rx="3.5" />
        <circle cx="12.5" cy="15" r="1.6" fill="#ffffff" stroke="none" />
        <circle cx="19.5" cy="15" r="1.6" fill="#ffffff" stroke="none" />
        <line x1="13" y1="19.5" x2="19" y2="19.5" />
      </>
    )),
  },
  {
    key: "nova",
    label: "Nova",
    Node: makeIcon("#6366f1", (
      <path
        d="M16 3 L19.2 12.8 L29 16 L19.2 19.2 L16 29 L12.8 19.2 L3 16 L12.8 12.8 Z"
        fill="#ffffff"
        stroke="none"
      />
    )),
  },
  {
    key: "spark",
    label: "Spark",
    Node: makeIcon("#f59e0b", (
      <>
        <path
          d="M17 4 L19 13 L28 15 L19 17 L17 26 L15 17 L6 15 L15 13 Z"
          fill="#ffffff"
          stroke="none"
        />
        <circle cx="25" cy="24" r="1.4" fill="#ffffff" stroke="none" />
        <circle cx="7" cy="24" r="1.1" fill="#ffffff" stroke="none" />
      </>
    )),
  },
  {
    key: "orbit",
    label: "Orbit",
    Node: makeIcon("#0ea5e9", (
      <>
        <circle cx="16" cy="16" r="5.5" fill="#ffffff" stroke="none" />
        <ellipse cx="16" cy="16" rx="13" ry="5" transform="rotate(-30 16 16)" />
      </>
    )),
  },
  {
    key: "prism",
    label: "Prism",
    Node: makeIcon("#8b5cf6", (
      <>
        <path d="M16 4 L27 25 L5 25 Z" />
        <path d="M16 4 L16 25" opacity="0.55" />
        <path d="M11 14 L21 14" opacity="0.55" />
      </>
    )),
  },
  {
    key: "echo",
    label: "Echo",
    Node: makeIcon("#14b8a6", (
      <>
        <path d="M9 11 Q14 16 9 21" />
        <path d="M13 8 Q20 16 13 24" />
        <path d="M17 5 Q26 16 17 27" />
      </>
    )),
  },
  {
    key: "pixel",
    label: "Pixel",
    Node: makeIcon("#f43f5e", (
      <>
        <rect x="8" y="8" width="6" height="6" rx="1" fill="#ffffff" stroke="none" />
        <rect x="18" y="8" width="6" height="6" rx="1" fill="#ffffff" stroke="none" />
        <rect x="8" y="18" width="6" height="6" rx="1" fill="#ffffff" stroke="none" />
        <rect x="18" y="18" width="6" height="6" rx="1" fill="#ffffff" stroke="none" />
      </>
    )),
  },
  {
    key: "vector",
    label: "Vector",
    Node: makeIcon("#10b981", (
      <>
        <path d="M16 4 L25 25 L16 20 L7 25 Z" fill="#ffffff" stroke="none" />
      </>
    )),
  },
  {
    key: "node",
    label: "Node",
    Node: makeIcon("#3b82f6", (
      <>
        <line x1="9" y1="9" x2="23" y2="16" />
        <line x1="9" y1="9" x2="11" y2="24" />
        <line x1="23" y1="16" x2="11" y2="24" />
        <circle cx="9" cy="9" r="2.6" fill="#ffffff" stroke="none" />
        <circle cx="23" cy="16" r="2.6" fill="#ffffff" stroke="none" />
        <circle cx="11" cy="24" r="2.6" fill="#ffffff" stroke="none" />
      </>
    )),
  },
  {
    key: "glyph",
    label: "Glyph",
    Node: makeIcon("#f97316", (
      <path
        d="M16 5 L25 27 H20.5 L18.8 22.5 H13.2 L11.5 27 H7 Z M14.4 18.7 H17.6 L16 14.2 Z"
        fill="#ffffff"
        stroke="none"
      />
    )),
  },
  {
    key: "wave",
    label: "Wave",
    Node: makeIcon("#06b6d4", (
      <path d="M3 16 Q7 7 11 16 T19 16 T29 16" />
    )),
  },
  {
    key: "flux",
    label: "Flux",
    Node: makeIcon("#d946ef", (
      <path
        d="M18 3 L7 18 H14.5 L13 29 L25 13 H17.5 Z"
        fill="#ffffff"
        stroke="none"
      />
    )),
  },
  {
    key: "cipher",
    label: "Cipher",
    Node: makeIcon("#78716c", (
      <>
        <circle cx="11" cy="16" r="4.5" />
        <line x1="15.5" y1="16" x2="27" y2="16" />
        <line x1="22" y1="16" x2="22" y2="20.5" />
        <line x1="25.5" y1="16" x2="25.5" y2="19.5" />
      </>
    )),
  },
  {
    key: "atlas",
    label: "Atlas",
    Node: makeIcon("#84cc16", (
      <>
        <circle cx="16" cy="16" r="10.5" />
        <ellipse cx="16" cy="16" rx="4.5" ry="10.5" />
        <line x1="5.5" y1="16" x2="26.5" y2="16" />
      </>
    )),
  },
  {
    key: "helix",
    label: "Helix",
    Node: makeIcon("#a855f7", (
      <>
        <path d="M10 4 Q22 11 10 18 Q-2 25 10 28" />
        <path d="M22 4 Q10 11 22 18 Q34 25 22 28" />
        <line x1="11" y1="8" x2="21" y2="8" opacity="0.7" />
        <line x1="9" y1="16" x2="23" y2="16" opacity="0.7" />
        <line x1="11" y1="24" x2="21" y2="24" opacity="0.7" />
      </>
    )),
  },
  {
    key: "pulse",
    label: "Pulse",
    Node: makeIcon("#ef4444", (
      <polyline points="3,17 10,17 13,9 16,23 19,14 22,17 29,17" />
    )),
  },
];

const ICON_BY_KEY: Record<string, AgentIconEntry> = Object.fromEntries(
  AGENT_ICONS.map((entry) => [entry.key, entry]),
);

/** Looks up a registry entry by key. `undefined` if the key is not registered. */
export function agentIconByKey(key: string | null | undefined): AgentIconEntry | undefined {
  if (!key) return undefined;
  return ICON_BY_KEY[key];
}

/**
 * Renders a registry icon by key, or `null` for an unknown / empty key. Used
 * by renderers that already know the key is valid; callers that need the
 * fallback for unknown keys do the lookup themselves (see `ActorAvatarBase`).
 */
export function AgentIcon({
  iconKey,
  size,
  className,
  title,
}: {
  iconKey?: string | null;
  size?: number;
  className?: string;
  title?: string;
}) {
  const entry = agentIconByKey(iconKey);
  if (!entry) return null;
  const { Node } = entry;
  return <Node size={size} className={className} title={title} />;
}
