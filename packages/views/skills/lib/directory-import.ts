const MAX_FILE_SIZE = 1 << 20; // 1 MiB per file
const MAX_TOTAL_SIZE = 8 << 20; // 8 MiB total supporting files
const MAX_FILE_COUNT = 128;

const LIKELY_BINARY_EXTENSIONS = new Set([
  ".png",
  ".jpg",
  ".jpeg",
  ".gif",
  ".webp",
  ".bmp",
  ".tiff",
  ".ico",
  ".heic",
  ".ttf",
  ".otf",
  ".woff",
  ".woff2",
  ".eot",
  ".zip",
  ".gz",
  ".tar",
  ".bz2",
  ".7z",
  ".rar",
  ".pdf",
  ".docx",
  ".xlsx",
  ".pptx",
  ".doc",
  ".xls",
  ".ppt",
  ".mp3",
  ".mp4",
  ".wav",
  ".avi",
  ".mov",
  ".webm",
  ".m4a",
  ".flac",
  ".exe",
  ".dll",
  ".so",
  ".dylib",
  ".class",
  ".jar",
  ".wasm",
  ".db",
  ".sqlite",
  ".sqlite3",
  ".pyc",
]);

export interface DirectorySkillFile {
  path: string;
  content: string;
}

export interface DirectorySkillImportResult {
  /** Skill name derived from frontmatter or the directory slug. */
  name: string;
  /** Skill description extracted from frontmatter when present. */
  description?: string;
  /** SKILL.md content. */
  content: string;
  /** Supporting files relative to the skill root. */
  files: DirectorySkillFile[];
  /** Files that were skipped (binary, oversized, unreadable, or duplicate SKILL.md). */
  skipped: string[];
  /** Human-readable errors that should be surfaced to the user. */
  errors: string[];
}

function isLikelyBinaryPath(path: string): boolean {
  const ext = path.slice(((path.lastIndexOf(".") - 1) >>> 0) + 2).toLowerCase();
  return LIKELY_BINARY_EXTENSIONS.has(ext);
}

function isReservedContentPath(path: string): boolean {
  // Mirrors server/internal/skill.IsReservedContentPath: case-insensitive
  // comparison against the cleaned path, so "./SKILL.md" also matches.
  const cleaned = path.replace(/\\/g, "/").split("/").filter(Boolean).join("/");
  return cleaned.toLowerCase() === "skill.md";
}

function findSkillRoot(files: File[]): { root: string; skillFile: File } | null {
  const candidates: { root: string; depth: number; file: File }[] = [];

  for (const file of files) {
    const relative = file.webkitRelativePath || file.name;
    const parts = relative.replace(/\\/g, "/").split("/").filter(Boolean);
    if (parts.length === 0) continue;
    const fileName = parts[parts.length - 1]!;
    if (fileName.toLowerCase() !== "skill.md") continue;
    // Root is everything before SKILL.md, e.g. ["my-skill"]
    const rootParts = parts.slice(0, -1);
    candidates.push({ root: rootParts.join("/"), depth: rootParts.length, file });
  }

  if (candidates.length === 0) return null;
  // Prefer the shallowest SKILL.md.
  candidates.sort((a, b) => a.depth - b.depth);
  const chosen = candidates[0]!;
  return { root: chosen.root, skillFile: chosen.file };
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

function parseFrontmatter(content: string): { name?: string; description?: string } {
  const result: { name?: string; description?: string } = {};
  if (!content.startsWith("---")) return result;
  const end = content.indexOf("\n---", 3);
  if (end === -1) return result;
  const block = content.slice(3, end);
  for (const line of block.split("\n")) {
    const trimmed = line.trim();
    const nameMatch = /^name:\s*(.+)$/.exec(trimmed);
    if (nameMatch?.[1]) {
      result.name = nameMatch[1].trim().replace(/^["']|["']$/g, "");
      continue;
    }
    const descMatch = /^description:\s*(.+)$/.exec(trimmed);
    if (descMatch?.[1]) {
      result.description = descMatch[1].trim().replace(/^["']|["']$/g, "");
    }
  }
  return result;
}

function normalizePath(relative: string, root: string): string {
  let clean = relative.replace(/\\/g, "/");
  if (root && clean.startsWith(`${root}/`)) {
    clean = clean.slice(root.length + 1);
  }
  return clean.replace(/^\.\//, "");
}

/**
 * Reads a user-selected directory and turns it into a Skill payload.
 *
 * - Finds the shallowest SKILL.md as the primary content.
 * - Supporting files keep their relative paths, including nested directories.
 * - Binary files and oversized files are skipped and reported.
 * - Enforces per-file and total-size caps matching the server-side importer.
 */
export async function readSkillDirectory(
  files: File[],
): Promise<DirectorySkillImportResult> {
  const result: DirectorySkillImportResult = {
    name: "",
    content: "",
    files: [],
    skipped: [],
    errors: [],
  };

  if (files.length === 0) {
    result.errors.push("No files selected.");
    return result;
  }

  const rootInfo = findSkillRoot(files);
  if (!rootInfo) {
    result.errors.push("Directory must contain a SKILL.md file.");
    return result;
  }

  const { root, skillFile } = rootInfo;

  // Use the directory slug as the fallback name.
  const directorySlug = root
    ? root.replace(/\\/g, "/").split("/").filter(Boolean).pop() ?? skillFile.name
    : skillFile.name;

  if (skillFile.size > MAX_FILE_SIZE) {
    result.errors.push(`SKILL.md exceeds ${MAX_FILE_SIZE >> 20} MiB.`);
    return result;
  }

  result.content = await readFileAsText(skillFile);
  const frontmatter = parseFrontmatter(result.content);
  result.name = frontmatter.name?.trim() || directorySlug;
  result.description = frontmatter.description?.trim();

  let totalSize = 0;
  const seenPaths = new Set<string>();

  for (const file of files) {
    const relative = file.webkitRelativePath || file.name;
    const normalized = normalizePath(relative, root);

    if (!normalized || isReservedContentPath(normalized)) {
      // Skip the primary SKILL.md and any other reserved paths.
      if (file !== skillFile) {
        result.skipped.push(normalized || file.name);
      }
      continue;
    }

    if (isLikelyBinaryPath(normalized)) {
      result.skipped.push(normalized);
      continue;
    }

    if (file.size > MAX_FILE_SIZE) {
      result.skipped.push(`${normalized} (exceeds 1 MiB)`);
      continue;
    }

    if (result.files.length >= MAX_FILE_COUNT) {
      result.skipped.push(`${normalized} (file limit reached)`);
      continue;
    }

    if (totalSize + file.size > MAX_TOTAL_SIZE) {
      result.skipped.push(`${normalized} (total size limit reached)`);
      continue;
    }

    if (seenPaths.has(normalized)) {
      result.skipped.push(`${normalized} (duplicate path)`);
      continue;
    }
    seenPaths.add(normalized);

    try {
      const content = await readFileAsText(file);
      result.files.push({ path: normalized, content });
      totalSize += file.size;
    } catch {
      result.skipped.push(`${normalized} (failed to read as text)`);
    }
  }

  return result;
}
