import { parseYamlDocument } from "@multica/core/skills";
import {
  readSkillDirectory,
  type DirectorySkillImportResult,
} from "../../skills/lib/directory-import";

/**
 * Agentwaker agent-role directory import.
 *
 * A standard agentwaker role directory looks like:
 *
 *   {role-name}/
 *   ├── agent-detail.en.md      ← instructions (soul mirror)
 *   ├── agent-soul/             ← skipped (content already in agent-detail.en.md)
 *   │   └── PROFILE.yaml        ← parsed for name / description / skills path
 *   ├── {role-name}-skills/     ← skill package
 *   │   ├── SKILL.md            ← meta entrypoint (skipped as a skill)
 *   │   └── {skill-id}/
 *   │       └── SKILL.md        ← one importable skill
 *   ├── env/
 *   │   ├── .env                ← preferred when present
 *   │   └── .env.example        ← fallback
 *   └── mcp/
 *       └── mcp.json            ← optional MCP server config
 *
 * `readAgentDirectory` turns a user-selected directory of files into a payload
 * that the import form turns into a `createAgent` call + N `createSkill` calls.
 */

// Mirrors the caps in skills/lib/directory-import.ts — every skill we materialize
// goes through `readSkillDirectory`, which already enforces per-skill limits, so
// we only cap the aggregate here.
const MAX_AGENT_INSTRUCTIONS_SIZE = 1 << 20; // 1 MiB — same ceiling as a skill file

/** Directories / files that are never useful as agent or skill content. */
const SKIP_DIR_NAMES = new Set(["__pycache__", ".git", "node_modules", ".idea"]);

interface ProfileYaml {
  display_name?: string;
  id?: string;
  mission?: string;
  skills?: {
    directory?: string;
    meta_entrypoint?: string;
  };
}

export interface AgentDirectoryImportResult {
  /** Agent display name — from PROFILE.display_name, falling back to the dir slug. */
  name: string;
  /** Agent description — from PROFILE.mission (one-line). */
  description: string;
  /** Full agent-detail.en.md content, used as the agent instructions. */
  instructions: string;
  /** Environment variables keyed by name. Values are always strings (empty when unset). */
  customEnv: Record<string, string>;
  /** Parsed mcp.json, or null when the file is absent / empty / invalid. */
  mcpConfig: unknown | null;
  /** One entry per importable sub-skill under the skills package directory. */
  skills: DirectorySkillImportResult[];
  /** Paths that were skipped (excluded dirs, the meta SKILL.md, etc). */
  skipped: string[];
  /** Human-readable errors that should be surfaced to the user. */
  errors: string[];
}

// ---------------------------------------------------------------------------
// File helpers
// ---------------------------------------------------------------------------

function relativePath(file: File): string {
  return (file.webkitRelativePath || file.name).replace(/\\/g, "/");
}

/** Returns the parts of a path relative to `root`, or null if not under root. */
function partsUnder(rel: string, root: string): string[] | null {
  const parts = rel.split("/").filter(Boolean);
  const rootParts = root.split("/").filter(Boolean);
  if (parts.length <= rootParts.length) return null;
  for (let i = 0; i < rootParts.length; i++) {
    if (parts[i] !== rootParts[i]) return null;
  }
  return parts.slice(rootParts.length);
}

function readFileAsText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      if (reader.result instanceof ArrayBuffer) {
        const decoder = new TextDecoder("utf-8", { fatal: true });
        try {
          resolve(decoder.decode(reader.result));
        } catch {
          reject(new Error(`not valid UTF-8: ${file.name}`));
        }
      } else {
        resolve(reader.result ?? "");
      }
    };
    reader.onerror = () => reject(new Error(`failed to read: ${file.name}`));
    reader.readAsText(file);
  });
}

/** Finds the shallowest file named `targetName` (case-insensitive) and returns
 *  its directory root (everything before the file). */
function findFileRoot(
  files: File[],
  targetName: string,
): { root: string; file: File } | null {
  const candidates: { root: string; depth: number; file: File }[] = [];
  const lowerTarget = targetName.toLowerCase();
  for (const file of files) {
    const parts = relativePath(file).split("/").filter(Boolean);
    if (parts.length === 0) continue;
    if (parts[parts.length - 1]!.toLowerCase() !== lowerTarget) continue;
    const rootParts = parts.slice(0, -1);
    candidates.push({ root: rootParts.join("/"), depth: rootParts.length, file });
  }
  if (candidates.length === 0) return null;
  candidates.sort((a, b) => a.depth - b.depth);
  const chosen = candidates[0]!;
  return { root: chosen.root, file: chosen.file };
}

function shouldSkipPath(parts: string[]): boolean {
  return parts.some((p) => SKIP_DIR_NAMES.has(p));
}

// ---------------------------------------------------------------------------
// .env parsing
// ---------------------------------------------------------------------------

/** Parses a dotenv-style file into a key→value map. Comment lines (`#`) and
 *  blank lines are ignored. A variable with no value maps to an empty string,
 *  matching the contract that imported env keys are placeholders for the user
 *  to fill in later. */
function parseEnvFile(content: string): Record<string, string> {
  const env: Record<string, string> = {};
  // `export KEY=value` is common; strip the leading `export `.
  const lineRe = /^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$/;
  for (const rawLine of content.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (line === "" || line.startsWith("#")) continue;
    const match = lineRe.exec(line);
    if (!match) continue;
    const key = match[1]!;
    let value = match[2] ?? "";
    // Strip a trailing inline comment (`KEY=value # note`). A `#` immediately
    // after `=` is kept as part of an empty value only when unquoted.
    if (!value.startsWith("#")) {
      const hashIdx = value.indexOf(" #");
      if (hashIdx !== -1) value = value.slice(0, hashIdx);
    }
    value = value.trim();
    // Unquote: "value" or 'value' → value.
    if (
      (value.startsWith('"') && value.endsWith('"')) ||
      (value.startsWith("'") && value.endsWith("'"))
    ) {
      value = value.slice(1, -1);
    }
    env[key] = value;
  }
  return env;
}

/** Picks the env file — prefers `.env`, falls back to `.env.example`. */
function pickEnvFile(files: File[], root: string): File | null {
  let example: File | null = null;
  for (const file of files) {
    const parts = partsUnder(relativePath(file), root);
    if (!parts) continue;
    if (parts.length !== 2 || parts[0] !== "env") continue;
    if (parts[1] === ".env") return file;
    if (parts[1] === ".env.example") example = file;
  }
  return example;
}

// ---------------------------------------------------------------------------
// mcp.json parsing
// ---------------------------------------------------------------------------

function findMcpJson(files: File[], root: string): File | null {
  for (const file of files) {
    const parts = partsUnder(relativePath(file), root);
    if (!parts) continue;
    if (parts.length === 2 && parts[0] === "mcp" && parts[1] === "mcp.json") {
      return file;
    }
  }
  return null;
}

// ---------------------------------------------------------------------------
// Skill discovery
// ---------------------------------------------------------------------------

/**
 * Groups files by their immediate sub-directory under the skills package root,
 * so each group can be handed to `readSkillDirectory` independently. The meta
 * `SKILL.md` that sits directly under the package root is excluded — it is a
 * routing manifest, not an importable skill.
 */
function groupSkillFiles(
  files: File[],
  skillsRoot: string,
): { subDir: string; files: File[] }[] {
  const groups = new Map<string, File[]>();
  for (const file of files) {
    const parts = partsUnder(relativePath(file), skillsRoot);
    if (!parts || parts.length === 0) continue;
    if (shouldSkipPath(parts)) continue;
    // `SKILL.md` at the package root (parts.length === 1) is the meta entrypoint.
    if (parts.length === 1) continue;
    const subDir = parts[0]!;
    if (!groups.has(subDir)) groups.set(subDir, []);
    groups.get(subDir)!.push(file);
  }
  return [...groups.entries()].map(([subDir, groupFiles]) => ({ subDir, files: groupFiles }));
}

/** Locates the skills package directory. Prefers PROFILE.yaml `skills.directory`;
 *  falls back to the sole top-level dir under root whose name ends in `-skills`. */
function findSkillsRoot(files: File[], root: string, profile: ProfileYaml | null): string | null {
  const declared = profile?.skills?.directory?.trim();
  if (declared) {
    // PROFILE stores a trailing slash; join with root.
    const cleaned = declared.replace(/\/+$/, "");
    const candidate = root ? `${root}/${cleaned}` : cleaned;
    if (files.some((f) => relativePath(f).startsWith(`${candidate}/`))) {
      return candidate;
    }
  }
  // Fallback: a top-level child of root whose name ends with `-skills`.
  const rootPrefix = root ? `${root}/` : "";
  const candidates = new Set<string>();
  for (const file of files) {
    const rel = relativePath(file);
    if (!rel.startsWith(rootPrefix)) continue;
    const rest = rel.slice(rootPrefix.length);
    const firstSeg = rest.split("/")[0];
    if (firstSeg && firstSeg.endsWith("-skills")) candidates.add(root ? `${root}/${firstSeg}` : firstSeg);
  }
  if (candidates.size === 1) return [...candidates][0]!;
  return null;
}

// ---------------------------------------------------------------------------
// Main entry
// ---------------------------------------------------------------------------

/**
 * Reads a user-selected agentwaker role directory and turns it into an agent
 * import payload: instructions, env, MCP config, and zero or more skills.
 */
export async function readAgentDirectory(
  files: File[],
): Promise<AgentDirectoryImportResult> {
  const result: AgentDirectoryImportResult = {
    name: "",
    description: "",
    instructions: "",
    customEnv: {},
    mcpConfig: null,
    skills: [],
    skipped: [],
    errors: [],
  };

  if (files.length === 0) {
    result.errors.push("No files selected.");
    return result;
  }

  // --- 1. Locate the role root via agent-detail.en.md ---
  const detailRoot = findFileRoot(files, "agent-detail.en.md");
  if (!detailRoot) {
    result.errors.push("Directory must contain an agent-detail.en.md file.");
    return result;
  }
  const { root, file: detailFile } = detailRoot;

  // Directory slug is the fallback name.
  const dirSlug = root ? root.split("/").filter(Boolean).pop() ?? "agent" : "agent";

  if (detailFile.size > MAX_AGENT_INSTRUCTIONS_SIZE) {
    result.errors.push(`agent-detail.en.md exceeds ${MAX_AGENT_INSTRUCTIONS_SIZE >> 20} MiB.`);
    return result;
  }

  try {
    result.instructions = await readFileAsText(detailFile);
  } catch (err) {
    result.errors.push(
      `Failed to read agent-detail.en.md: ${err instanceof Error ? err.message : "unknown error"}`,
    );
    return result;
  }

  // --- 2. Parse PROFILE.yaml for name / description / skills path ---
  let profile: ProfileYaml | null = null;
  const profileFile = findFileRoot(files, "PROFILE.yaml");
  if (profileFile) {
    // PROFILE.yaml lives under agent-soul/, so its root differs from `root`.
    // We only need the file content, not its location.
    try {
      const profileText = await readFileAsText(profileFile.file);
      profile = parseYamlDocument<ProfileYaml>(profileText);
    } catch {
      // Non-fatal: name / description fall back to the dir slug.
    }
  }

  result.name = profile?.display_name?.trim() || dirSlug;
  // `mission` is a YAML folded scalar; collapse to a single line for the
  // description field, which has a 255-char ceiling downstream.
  const mission = profile?.mission?.trim().replace(/\s+/g, " ") ?? "";
  result.description = mission.slice(0, 255);

  // --- 3. Parse env (.env preferred, .env.example fallback) ---
  const envFile = pickEnvFile(files, root);
  if (envFile) {
    try {
      const envText = await readFileAsText(envFile);
      result.customEnv = parseEnvFile(envText);
    } catch {
      // Non-fatal: agent still imports without env.
    }
  }

  // --- 4. Parse mcp.json ---
  const mcpFile = findMcpJson(files, root);
  if (mcpFile) {
    try {
      const mcpText = (await readFileAsText(mcpFile)).trim();
      if (mcpText) {
        const parsed = JSON.parse(mcpText);
        // Only keep it when it's a non-empty object (e.g. { "mcpServers": {...} }).
        // An empty `{ "mcpServers": {} }` means "no servers configured" — skip it
        // so we don't overwrite any runtime default.
        if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
          const servers = (parsed as { mcpServers?: unknown }).mcpServers;
          const hasServers =
            servers != null &&
            typeof servers === "object" &&
            !Array.isArray(servers) &&
            Object.keys(servers as Record<string, unknown>).length > 0;
          if (hasServers) {
            result.mcpConfig = parsed;
          }
        }
      }
    } catch {
      result.errors.push("mcp/mcp.json is not valid JSON — skipped.");
    }
  }

  // --- 5. Discover and parse skills ---
  const skillsRoot = findSkillsRoot(files, root, profile);
  if (!skillsRoot) {
    // No skills package is valid (some roles are instruction-only).
    return result;
  }

  const groups = groupSkillFiles(files, skillsRoot);
  for (const { subDir, files: skillFiles } of groups) {
    try {
      const skillResult = await readSkillDirectory(skillFiles);
      if (skillResult.errors.length > 0) {
        result.skipped.push(`${subDir}/ (${skillResult.errors.join("; ")})`);
        continue;
      }
      // readSkillDirectory derives `name` from frontmatter or the directory slug.
      // Ensure we never import a skill with an empty name.
      if (!skillResult.name.trim()) {
        skillResult.name = subDir;
      }
      result.skills.push(skillResult);
    } catch (err) {
      result.skipped.push(
        `${subDir}/ (failed: ${err instanceof Error ? err.message : "unknown error"})`,
      );
    }
  }

  return result;
}
