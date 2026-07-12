/**
 * AgentWaker directory integration types.
 *
 * A configured AgentWaker source directory is scanned by the owning daemon;
 * Multica stores sanitized snapshots and (in M2+) applies them atomically. The
 * types here mirror the server-side handler response shapes. Plaintext env
 * values never appear in any of these types — only key names, configured
 * booleans, and value digests.
 */

export type AgentSourceKind = "agentwaker_directory";
export type AgentSourceSyncMode = "manual" | "scheduled" | "watch-assisted";
export type AgentSourceStatus =
  | "pending"
  | "scanning"
  | "ready"
  | "applying"
  | "partial"
  | "failed"
  | "offline";

export interface AgentSource {
  id: string;
  workspace_id: string;
  kind: AgentSourceKind;
  daemon_runtime_id: string;
  local_path: string;
  sync_mode: AgentSourceSyncMode;
  status: AgentSourceStatus;
  last_snapshot_hash?: string;
  last_scanned_at?: string;
  last_applied_at?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateAgentSourceRequest {
  daemon_runtime_id: string;
  local_path: string;
  sync_mode?: AgentSourceSyncMode;
}

export interface UpdateAgentSourceRequest {
  daemon_runtime_id?: string;
  local_path?: string;
  sync_mode?: AgentSourceSyncMode;
}

// --- scan request (the in-flight record polled until terminal) ---

export type AgentWakerScanStatus =
  | "pending"
  | "running"
  | "completed"
  | "failed"
  | "timeout";

export interface ScanDiagnostic {
  severity: "error" | "warning" | "info";
  code: string;
  message: string;
  path?: string;
}

export interface AgentWakerScanRequest {
  id: string;
  source_id: string;
  runtime_id: string;
  status: AgentWakerScanStatus;
  directory_hash?: string;
  /** Sanitized manifest; value-free (env carries value_digest, never value). */
  manifest?: unknown;
  diagnostics?: ScanDiagnostic[];
  scanner_version?: string;
  error?: string;
  created_at: string;
  updated_at: string;
}

// --- snapshot (immutable scan/apply record) ---

export type AgentSourceSnapshotStatus = "preview" | "applied" | "failed" | "superseded";

export interface AgentSourceSnapshot {
  id: string;
  source_id: string;
  directory_hash: string;
  schema_versions: Record<string, unknown>;
  /** Sanitized manifest; value-free. */
  manifest: unknown;
  status: AgentSourceSnapshotStatus;
  diagnostics: ScanDiagnostic[];
  lock_yaml?: string;
  scanner_version?: string;
  created_at: string;
  applied_at?: string;
}

// --- sanitized manifest value-safe shapes (for preview UI) ---

export interface SanitizedEnvDeclaration {
  name: string;
  required: boolean;
  description?: string;
  configured: boolean;
  secret: boolean;
  value_digest?: string;
}

export interface SanitizedCapabilitySummary {
  id: string;
  name: string;
  version: string;
  description: string;
  entrypoint: string;
  content_hash: string;
  profile_count: number;
  adapter_count: number;
  permissions: {
    default_mode: "read-only" | "local-write" | "external-write";
    supports_account_actions: boolean;
  };
}

export interface SanitizedCapabilityBinding {
  id: string;
  version: string;
  required: boolean;
  used_by: { skill: string; profile: string }[];
  mode: "read-only" | "local-write" | "external-write";
  fallback: "continue" | "partial" | "blocked";
}

export interface SanitizedSkillSummary {
  id: string;
  name: string;
  is_meta: boolean;
  entrypoint: string;
  content_hash: string;
  file_count: number;
}

export interface SanitizedRoleSummary {
  id: string;
  role_dir: string;
  display_name: string;
  title: string;
  version: string;
  lifecycle: string;
  mission: string;
  instructions_hash: string;
  persona_hash: string;
  skills: SanitizedSkillSummary[];
  capability_bindings: SanitizedCapabilityBinding[];
  env: SanitizedEnvDeclaration[];
  mcp: {
    has_servers: boolean;
    server_count: number;
    unresolved_env?: string[];
  };
}

export interface SanitizedScanManifest {
  capabilities: SanitizedCapabilitySummary[];
  roles: SanitizedRoleSummary[];
}
