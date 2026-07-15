# AgentWaker Directory Integration Plan

Status: proposed  
Target repository: `multica`  
Source contract: AgentWaker Schema 2.1 plus shared-capability manifests 1.0

## Completeness Principle

This design is the required end state, not an MVP menu. Delivery milestones may sequence implementation and verification, but they must not replace the complete model with temporary product architecture. In particular:

- implement the full source, snapshot, capability-version, role, skill, binding, environment-secret, artifact, lock, sync, audit, and rollback model;
- do not model shared capabilities as ordinary skills, use display names as source identity, or orchestrate imports through multiple frontend API calls;
- do not ship plaintext centralized secrets as an accepted interim state;
- do not stop at one-time import: continuous detection, versioned updates, dependency re-resolution, rollback, and last-known-good behavior are part of completion;
- do not silently skip declared binary assets: use a content-addressed artifact store with hashes, media type, size limits, authorization, and lifecycle references;
- milestones below are merge and validation boundaries for one coherent system, not permission to declare the feature complete early.

## Outcome

A workspace administrator configures one AgentWaker source directory and the daemon automatically scans, validates, previews, imports, and later re-syncs the complete directory:

- shared capabilities under `capabilities/`;
- every formal role under `*/agent-soul/PROFILE.yaml`;
- role-owned meta and specialist skills;
- role `capabilities.yaml` dependency bindings;
- `env/.env.example` variable declarations and `env/.env` configured values;
- `mcp/mcp.json` declarations;
- English instructions, Chinese profile HTML, role metadata, and source hashes.

Import order is deterministic:

1. scan and validate the source directory;
2. register or update shared capabilities;
3. import role-owned skills;
4. create or update roles as Multica agents;
5. bind role skills and shared capabilities;
6. securely apply environment declarations, uploaded `.env` values, and MCP configuration;
7. commit one source snapshot and expose the result in the UI.

The configured directory remains the source of truth. Multica stores imported copies and source identities; it does not turn generated database state into a second editable source tree.

## Current State

Multica already has useful pieces, but they currently form a browser-only, role-at-a-time workflow.

### Existing strengths

- `packages/views/agents/lib/agent-import.ts` reads one selected role directory.
- It imports `agent-detail.en.md`, `agent-detail.zh.md`, the text files linked by the localized detail page, the exact role-scoped `env/.env` body, `agent-persona.html`, `PROFILE.yaml`, role skills, environment data, and `mcp/mcp.json`. Source files are view-only presentation data; path traversal, binaries, symlinks, and unrelated dotenv files are excluded.
- `packages/views/agents/components/create-agent-dialog.tsx` and `overwrite-agent-dialog.tsx` create or overwrite skills and bind them through `agent_skill`.
- Skills already support a primary `SKILL.md`, supporting files, provenance in `skill.config`, full-file replacement, and bundle hashing.
- The daemon/server protocol already supports queued local-skill listing and import work through heartbeat requests and async results.
- The daemon already validates daemon-owned absolute local directories for project resources.

### Gaps

1. The browser must select one role directory manually; Multica cannot configure and rescan the AgentWaker repository root.
2. Parsing and orchestration live in React view code, so CLI, API, daemon, and scheduled sync cannot reuse them.
3. The current import is not one transaction. Agent update, environment update, N skill writes, and bindings can partially succeed.
4. Same-name matching is used as identity. Renames can create duplicates or overwrite unrelated workspace skills.
5. Shared `capabilities/`, `CAPABILITY.yaml`, registry entries, role `capabilities.yaml`, profiles, and version constraints are not understood.
6. No source snapshot, digest, sync status, drift report, dependency lock, or rollback record exists.
7. The current browser parser can read `env/.env`, but it has no directory-source provenance, preview/apply separation, or source-sync policy. The new flow packages the exact body for the detail source viewer and parses it during explicit apply for encrypted storage; unrelated dotenv files remain excluded.
8. `setAgentSkills` is replace-all. A directory sync needs explicit ownership so it removes only bindings managed by that source and does not silently detach user-managed skills.
9. Binary assets are skipped by the current text-only skill storage. Capability packages need an explicit artifact policy rather than silent loss.

## Architectural Decision

Directory scanning belongs to the daemon, not the browser or remote server.

```text
Web / CLI
  configure source {daemon_id, absolute_path, sync_mode}
                    |
                    v
Multica Server
  stores source, queues scan, receives manifest, validates plan,
  applies atomic import, stores snapshot and bindings
                    |
                    v
Selected Multica Daemon
  owns filesystem access, canonicalizes path, scans files,
  parses AgentWaker contracts, hashes bundles, returns sanitized manifest
```

Reasons:

- the source path is local to a particular machine or NAS;
- the server may be remote and must not assume it can read the path;
- browser directory handles are not durable or portable;
- daemon path validation and heartbeat work queues already exist;
- secret classification and redaction must happen before a scan preview leaves the source machine; actual `.env` values travel only in the explicit apply payload.

Do not implement this as a larger `<input webkitdirectory>` parser. Keep manual single-role import as a separate convenience flow until directory sync reaches parity, then decide whether to remove it.

## Source Contracts

### Repository discovery

The scanner accepts a configured absolute root only when all of these are true:

- `capabilities/registry.yaml` exists;
- `schemas/profile-v2.1.schema.json` exists;
- at least one `*/agent-soul/PROFILE.yaml` exists;
- the path passes the existing daemon local-directory canonical-path, ownership, symlink, and forbidden-root checks.

Ignore at minimum:

- `.git`, `node_modules`, `.idea`, runtime `workdir` contents, caches, archives, temporary files;
- unrelated `.env` files and credential stores outside each recognized role's exact `env/.env` path;
- untracked runtime artifacts that are not part of a declared skill or capability contract;
- unrelated binary files; declared capability or skill assets enter the content-addressed artifact pipeline rather than being silently skipped.

### Shared capability

Read:

- `capabilities/registry.yaml`;
- `capabilities/{id}/CAPABILITY.yaml`;
- declared `entrypoint`;
- declared input and output schemas;
- textual supporting files required by the package.

Identity is `(source_id, capability_id)`, not display name. Version and content hash are separate fields.

### Role

Read:

- `agent-soul/PROFILE.yaml` as machine identity and routing metadata;
- `agent-detail.en.md` as runtime instructions;
- `agent-persona.html` as profile HTML;
- the skill directory declared by `PROFILE.yaml`;
- `capabilities.yaml` as the role-skill-to-shared-capability dependency manifest;
- `env/.env.example` for declarations and `env/.env` for the actual values to synchronize;
- `mcp/mcp.json` after validating every `${ENV_NAME}` reference against declared environment keys.

`agent-detail.en.md` remains the imported instruction body for compatibility. The scanner should also hash the ten authoritative `agent-soul` files so drift is detectable even when the aggregate was not regenerated.

## Proposed Data Model

The entities and relationships below are required. Physical names may follow existing migration conventions, but implementations must not collapse or omit their distinct responsibilities.

### `agent_source`

One configured AgentWaker directory.

| Column | Purpose |
| --- | --- |
| `id`, `workspace_id` | tenant identity |
| `kind` | `agentwaker_directory`, with an extensible source-kind constraint |
| `daemon_runtime_id` | daemon that owns the filesystem path |
| `local_path` | absolute configured path; return cautiously in APIs |
| `canonical_path_hash` | stable comparison without exposing the full path everywhere |
| `sync_mode` | `manual`, `scheduled`, or `watch-assisted`; every automatic mode still performs a canonical full rescan before planning/apply |
| `status` | `pending`, `scanning`, `ready`, `applying`, `partial`, `failed`, `offline` |
| `last_snapshot_hash` | last successfully applied directory hash |
| `last_scanned_at`, `last_applied_at` | audit timestamps |
| `created_by`, timestamps | ownership and audit |

### `agent_source_snapshot`

Immutable scan/apply record.

| Column | Purpose |
| --- | --- |
| `id`, `source_id` | snapshot identity |
| `directory_hash` | canonical manifest hash |
| `schema_versions` | detected AgentWaker contracts |
| `manifest` | sanitized parsed manifest with env key names, configured state, and value digests, but no raw `.env` values |
| `status` | `preview`, `applied`, `failed`, `superseded` |
| `diagnostics` | validation errors and warnings |
| `created_at`, `applied_at` | audit |

### `shared_capability`

Workspace-installed shared capability package.

| Column | Purpose |
| --- | --- |
| `id`, `workspace_id` | Multica identity |
| `source_id`, `source_key` | stable source identity such as `information-collection` |
| `name`, `version`, `description` | manifest metadata |
| `content`, `files` | entrypoint and supporting text bundle, using the established skill-bundle rules where possible |
| `manifest`, `content_hash` | full capability contract and reproducibility |
| timestamps | lifecycle |

Do not model a shared capability as an ordinary role skill with a magic name. It has profiles, version constraints, permissions, and many-to-many consumers that ordinary `agent_skill` cannot represent.

### `agent_source_role`

Maps a source role ID to a Multica agent ID and last imported hash. This survives display-name changes.

### `agent_source_skill`

Maps `(source_id, role_id, skill_id)` to a Multica skill ID and records content hash and whether the skill is the role meta entrypoint or a specialist skill. Store AgentWaker origin metadata in `skill.config.origin` as a secondary display surface, not as the only relational identity.

### `agent_capability_binding`

| Column | Purpose |
| --- | --- |
| `agent_id` | consuming Multica agent |
| `role_skill_id` | consuming role-owned skill |
| `capability_id` | installed shared capability |
| `profile` | selected capability profile |
| `version_requirement` | source constraint such as `^1.0.0` |
| `required` | activation requirement |
| `permissions`, `fallback` | role-specific restriction and failure behavior |
| `source_snapshot_id` | provenance |

Effective permissions are the intersection of system policy, capability support, role declaration, and current user approval. Import must reject attempted expansion, not silently clamp it without diagnostics.

### Environment declarations and values

`.env.example` defines the expected variables; `.env` is the source of the actual role configuration and must be uploaded during apply. Do not put example placeholders into `agent.custom_env` as if they were configured values. Add a declaration surface, either a dedicated `agent_env_declaration` table or metadata associated with the source role:

- variable name;
- required/optional;
- description if recoverable from `.env.example` comments;
- configured boolean;
- secret boolean where known.

The structured environment preview contains key names, configured booleans, and a keyed or otherwise non-reversible value digest for change detection. Separately, the explicitly requested `source_files` package contains the exact role-scoped `env/.env` body for the detail viewer. Parsed values are sent to the apply endpoint and encrypted at rest before storage.

The current `agent.custom_env` column is plaintext JSONB at rest; the dedicated env API protects read access and auditing but does not provide database encryption. AgentWaker directory sync is intended to become centralized secret management, so synchronized values must be encrypted at rest using the existing application-layer `secretbox` infrastructure, with key identifiers and authenticated encryption. Task preparation decrypts only for the owning agent execution. Reusing plaintext `custom_env` as the final or interim synchronized-value store is not an acceptable completion path.

Merge policy must be configurable per source and visible in the apply plan:

- `source-authoritative` (recommended for this use case): `.env` replaces source-managed keys, adds new keys, and removes source-managed keys deleted from the file; user-managed keys outside source ownership remain untouched.
- `merge-preserve`: `.env` adds or updates keys but does not remove existing values.
- an empty value in `.env` is a real configured empty value; a missing key has different semantics.

Every environment mutation records keys and value-change digests in the audit log, never plaintext. List APIs, realtime events, plans, diagnostics, and structured environment previews expose only metadata. The agent detail source-files response is the narrow exception: it returns the exact `env/.env` body. Environment-management reveal remains owner/admin-only and audited.

## Daemon Protocol

Extend the existing heartbeat queue pattern used by runtime-local skills instead of creating an unrelated transport.

Suggested capability flag:

```text
agentwaker-directory-sync-v1
```

Suggested pending request:

```json
{
  "id": "request-uuid",
  "source_id": "source-uuid",
  "local_path": "/absolute/path/agentwaker",
  "expected_snapshot_hash": "optional-sha256",
  "mode": "scan"
}
```

Suggested result envelope:

```json
{
  "source_id": "source-uuid",
  "request_id": "request-uuid",
  "status": "ready",
  "directory_hash": "sha256:...",
  "manifest": {
    "capabilities": [],
    "roles": [],
    "files": []
  },
  "diagnostics": [],
  "scanner_version": "1"
}
```

The structured environment declarations and diagnostics are sanitized; the requested source-file package may contain the exact `env/.env` body. Apply also sends parsed `env_values`, bound to the selected source and snapshot, and the server seals them before database storage.

The server must authenticate that the reporting daemon owns the configured runtime and workspace. Apply size, count, UTF-8, path, and hash limits on both daemon and server boundaries.

Use content-addressed per-package and artifact resolution from the outset, extending the existing skill bundle cache pattern. The sanitized scan manifest carries metadata and hashes; the server resolves changed bundles and declared assets by hash. Do not embed the entire repository into one unbounded heartbeat result.

## Server API

Recommended endpoints:

```text
POST   /api/workspaces/{workspace_id}/agent-sources
GET    /api/workspaces/{workspace_id}/agent-sources
GET    /api/agent-sources/{source_id}
PATCH  /api/agent-sources/{source_id}
DELETE /api/agent-sources/{source_id}

POST   /api/agent-sources/{source_id}/scan
GET    /api/agent-sources/{source_id}/snapshots/{snapshot_id}
GET    /api/agent-sources/{source_id}/plan?from={hash}&to={hash}
POST   /api/agent-sources/{source_id}/apply
POST   /api/agent-sources/{source_id}/rollback
```

Daemon-only result endpoint follows the existing `/api/daemon/runtimes/.../{request}/result` convention.

Creation should default to `scan`, not immediate apply. The UI shows an import plan before the first write. Later manual resync may support an explicitly configured auto-apply policy, but destructive changes still require review.

## Import Plan and Atomic Apply

The server computes a plan from stable source IDs and hashes:

```text
capabilities: create / update / unchanged / incompatible
roles:        create / update / unchanged / archive-candidate
skills:       create / update / unchanged / remove-binding-candidate
bindings:     add / update / remove
env:          declare / add / update / remove / unchanged / missing-required
mcp:          update / unresolved-env / unchanged
```

Apply all database mutations in one transaction. Reuse transaction-aware skill helpers such as `createSkillWithFilesInTx`; add transaction-aware update and agent materialization helpers instead of orchestrating N public API calls from React.

Rules:

- source identity wins over same-name matching;
- a same-name row without matching source provenance is a conflict requiring an explicit action;
- unchanged hashes produce no writes;
- update preserves Multica IDs and unrelated runtime/history data;
- source-managed bindings may be replaced from the new snapshot;
- user-managed bindings remain untouched;
- source deletion defaults to `archive-candidate` or `detach-candidate`, never immediate destructive deletion;
- required capability failure prevents role activation but does not destroy the last good applied snapshot;
- the snapshot becomes `applied` only after the transaction commits.

## Skill Materialization

Keep the existing role-owned Skill model:

- import the meta `SKILL.md` as a role-owned routing skill;
- import each specialist directory as one ordinary Multica skill bundle;
- bind these skills to the role through `agent_skill`;
- record stable source mapping in `agent_source_skill`;
- include textual references, scripts, templates, schemas, and examples within existing bundle limits;
- report every excluded binary or oversized file.

Shared capabilities have one stable installed identity per `(workspace_id, source_id, capability_id)`, but their version and content are continuously synchronized from AgentWaker. A capability update preserves the Multica capability ID, creates a new immutable version snapshot, re-resolves every consuming binding, and updates the active version only after validation succeeds. At task preparation time, materialize:

1. the role-owned skills already selected for the agent;
2. each bound shared capability entrypoint and supporting files;
3. a generated, machine-owned binding note that tells the runtime which role skill uses which capability profile and restrictions.

Avoid copying the capability into every role-owned skill row. Content-addressed runtime materialization may copy files into an execution sandbox, but the database source package remains single-instance.

### Shared capability update lifecycle

Every rescan compares capability identity, semantic version, manifest, contracts, adapters, profiles, permissions, and content hash.

```text
AgentWaker capability changed
  -> daemon discovers new manifest and content hash
  -> server creates capability update plan
  -> resolve all role version requirements and selected profiles
  -> run compatibility and permission checks
  -> show affected roles and bindings in Preview
  -> Apply writes a new immutable capability version and binding resolution
  -> new tasks use the new active version
  -> old snapshot remains available for rollback and historical task evidence
```

Update rules:

- content changes without a version change are allowed to be detected but must produce a `version-not-bumped` warning; policy may block Apply in strict mode;
- compatible updates update all bindings whose version requirement accepts the new version;
- an incompatible update does not silently break consumers: affected roles become `blocked-by-update` in the plan until the role constraint/profile is updated or the prior capability version remains pinned;
- removed profiles or narrowed permissions list every affected role before Apply;
- capability rollback restores the previous active version and binding resolution without rewriting role-owned wrapper skills;
- already running tasks remain pinned to the capability bundle hashes captured when they were claimed; only new tasks use the newly active version;
- updating a shared capability never creates one editable copy per role.

## Lock and Reproducibility

After successful apply, generate a Multica-side lock representation:

```yaml
schema_version: "1.0"
source_snapshot: sha256:...
capabilities:
  information-collection:
    requested: ^1.0.0
    resolved: 1.0.0
    digest: sha256:...
```

The daemon scan is read-only and must not write the lock into the configured source directory. Store every resolved lock in the applied snapshot and expose it through API, UI, CLI download, comparison, and rollback evidence.

## Security

- Read only each recognized role's exact `{role}/env/.env`; never recursively treat unrelated `.env` files as role configuration.
- During scan, package only the exact role-scoped `.env` body in `source_files`; keep it out of diagnostics, logs, analytics, events, list APIs, and structured environment previews. During explicit authenticated apply, send parsed values and encrypt them at rest.
- Use TLS for daemon/server transport and bind secret apply payloads to source, snapshot, workspace, daemon, target role, nonce, and expiry. Prefer application-layer envelope encryption in addition to transport protection.
- Encrypt centrally managed values at rest with the existing `secretbox` infrastructure before calling the system a secure centralized configuration store; the current `custom_env` JSONB storage is plaintext.
- Reject symlink escapes and paths outside the canonical configured root.
- Never execute scripts while scanning or importing.
- Treat `SKILL.md`, scripts, MCP commands, and capability adapters as untrusted content; show a review summary before first apply.
- Validate MCP environment references. Missing variables are configuration blockers, not empty values to inject.
- Sanitize HTML exactly as the existing sandboxed profile view expects; importing profile HTML must not grant execution privileges.
- Enforce workspace membership and admin/owner permissions for source configuration and apply.
- Redact absolute paths from ordinary member-facing events where they are not needed.
- Keep last-known-good snapshot active when a new scan fails.

## Sync Policy

The complete system supports manual scan, scheduled hash polling, and watch-assisted change detection. Filesystem watcher events are hints only because they can be lost or behave differently on network filesystems. Debounce every hint and perform a canonical full manifest rescan before creating a plan. Automatic detection never bypasses validation, snapshot creation, dependency resolution, or destructive-change approval policy.

State machine:

```text
pending -> scanning -> ready -> applying -> synced
               |          |         |
               v          v         v
             failed     blocked    failed

daemon offline -> offline -> scanning after reconnect
```

## UI

Add an AgentWaker source section at workspace level, not inside one agent dialog.

Configuration fields:

- daemon/runtime selector;
- absolute AgentWaker root path;
- sync mode and schedule, including manual, scheduled, and watch-assisted detection;
- `Test and scan` action.

Preview shows:

- detected schema versions;
- number of capabilities, roles, role skills, environment declarations, and MCP servers;
- validation failures;
- per-role dependency graph;
- create/update/unchanged/conflict/archive-candidate plan;
- environment keys that will be added, changed, removed, or remain missing, without displaying values;
- skipped files and security warnings.

Actions:

- apply selected snapshot;
- rescan;
- compare with current applied snapshot;
- rollback to prior snapshot;
- detach source without deleting imported agents;
- archive source-managed removed roles after explicit confirmation and retain rollback evidence.

Follow repository boundaries: schemas and API parsing in `packages/core`, shared pages in `packages/views`, Next.js wiring only in `apps/web`.

## CLI

Recommended commands:

```bash
multica agent-source add \
  --type agentwaker-directory \
  --daemon-id <runtime-id> \
  --path /Users/.../agentwaker

multica agent-source scan <source-id> --wait --output json
multica agent-source plan <source-id> --output json
multica agent-source apply <source-id> --snapshot <snapshot-id> --wait
multica agent-source status <source-id>
multica agent-source rollback <source-id> --snapshot <snapshot-id>
```

If these commands or endpoint semantics are added, update the relevant built-in Multica skills and source maps in the same change, as required by `CLAUDE.md`.

## Delivery Milestones

The milestones establish build order and testable integration boundaries. The feature is complete only when all milestones pass end to end.

### Milestone 0: contract fixtures

- Add a small AgentWaker fixture containing two capabilities, one role with dependencies, one empty-dependency role, MCP env references, and skipped runtime files.
- Port only the needed AgentWaker schemas into test fixtures or implement compatible typed validation. Do not shell out to Ruby validators from production Go.
- Establish canonical hashing fixtures shared by Go tests and TypeScript display tests.

### Milestone 1: daemon scan and preview

- Add source tables and APIs.
- Extend heartbeat protocol with AgentWaker scan requests/results.
- Implement the read-only Go scanner with strict path rules, sanitized env metadata, and content-addressed package/artifact discovery.
- Store immutable snapshots and expose preview/diagnostics.
- Add workspace configuration and preview UI.
- Land this milestone behind a feature gate; it is an integration checkpoint, not a complete product release.

### Milestone 2: atomic import and encrypted configuration

- Add shared-capability and source-mapping tables.
- Implement plan generation and one-transaction apply.
- Materialize roles, role skills, bindings, env declarations, MCP, instructions, and profile HTML.
- Add conflict decisions and last-known-good semantics.
- Add CLI commands.
- Encrypt synchronized environment values at rest with key rotation metadata; integrate audited reveal and task-time decryption.

### Milestone 3: runtime capability and artifact injection

- Extend task skill references to include shared capability bundles once per task.
- Generate runtime binding metadata.
- Verify Claude, Codex, Cursor, OpenCode, and generic `.agent_context` materialization.
- Preserve existing skill hash/cache validation.
- Resolve declared binary assets through the content-addressed artifact store with authorization and integrity checks.

### Milestone 4: continuous resync, dependency upgrades, and rollback

- Hash-based no-op scans.
- Diff/plan UI and rollback.
- Scheduled polling and watch-assisted detection with debounce and canonical rescans.
- Explicit archive/detach handling for removed source objects.
- Shared-capability version upgrades, affected-role preview, compatibility blocking, task pinning, and capability rollback.

## Acceptance Criteria

1. Configuring the AgentWaker repository root through one daemon discovers all 12 current roles and both initial shared capabilities.
2. The scanner reads each recognized role's `env/.env`, packages its exact body for the detail source viewer, and emits sanitized structured declarations; explicit apply uploads parsed values for encrypted storage and runtime injection.
3. First scan performs no workspace mutation.
4. Applying a snapshot creates or updates all selected objects in one transaction.
5. Reapplying an unchanged snapshot produces no writes and preserves IDs.
6. Renaming a source role or skill updates the mapped Multica object rather than duplicating it.
7. Same-name unrelated workspace objects produce explicit conflicts.
8. Every accepted `information-collection` source update creates a new workspace capability version, preserves its stable identity, re-resolves all consuming bindings, and makes the new version available to new tasks without creating per-role copies.
9. Required incompatible or missing capabilities block only affected roles and preserve the last good snapshot.
10. User-added skills and non-source-managed environment values survive resync; source-managed environment keys follow the configured authoritative or merge-preserve policy.
11. Removed source roles are proposed for archive/detach, never silently deleted.
12. Daemon disconnect, invalid schema, symlink escape, oversized bundle, malformed MCP, and partial scan all have tests and visible diagnostics.
13. Runtime task preparation receives role skills plus resolved shared capabilities with the correct profile and restricted permissions.

## Implementation Prompt for a Development AI

Use the prompt below in a fresh Multica development task.

```text
You are implementing AgentWaker directory integration in the Multica repository at:
/Users/code2rich/home/share-project/multica

The source AgentWaker repository is:
/Users/code2rich/home/share-project/agentwaker

Read these files completely before changing code:
- AGENTS.md
- CLAUDE.md
- docs/agentwaker-directory-integration-plan.md
- packages/views/agents/lib/agent-import.ts
- packages/views/agents/components/create-agent-dialog.tsx
- packages/views/agents/components/overwrite-agent-dialog.tsx
- packages/views/skills/lib/directory-import.ts
- server/internal/handler/runtime_local_skills.go
- server/internal/handler/daemon.go sections for heartbeat pending work
- server/internal/daemon/client.go heartbeat/result methods
- server/internal/daemon/local_directory.go
- server/internal/daemon/local_skills.go
- server/internal/handler/skill_create.go
- server/pkg/skillbundle/hash.go
- server/pkg/db/queries/skill.sql
- server/pkg/protocol/messages.go
- the latest migrations

Also inspect the current AgentWaker contracts directly:
- capabilities/registry.yaml
- capabilities/*/CAPABILITY.yaml
- schemas/capability.schema.json
- schemas/role-capabilities.schema.json
- schemas/profile-v2.1.schema.json
- */capabilities.yaml
- */agent-soul/PROFILE.yaml

Goal:
Allow a workspace administrator to configure one daemon-owned absolute AgentWaker root directory, scan it without mutation, preview a validated import plan, then atomically import shared capabilities, every role, role-owned skills, role-capability bindings, `.env.example` declarations, securely uploaded `.env` values, MCP configuration, instructions, and profile HTML. The configured directory remains the source of truth and later rescans are idempotent by stable source identity and content hash.

Non-negotiable architecture:
1. Filesystem scanning runs in the selected daemon. The browser and remote server must not read the configured path directly.
2. Reuse the existing daemon heartbeat pending-request/result pattern used by runtime-local skills.
3. The first scan is read-only and creates an immutable scoped snapshot. Apply is a separate explicit action.
4. Read each recognized role's exact `env/.env` because it is both a requested source document and the centralized configuration source. Package its body only in scoped source files, upload parsed values through explicit authenticated apply, encrypt them at rest, and inject them into the owning agent runtime.
5. Never execute imported scripts during scan or apply.
6. Use stable source IDs, not names, for role/skill/capability identity.
7. Apply all database changes in one transaction. Do not orchestrate N API calls from React.
8. Shared capabilities have one stable workspace/source identity but are continuously versioned and updated on resync. Re-resolve every consuming role binding after each update; do not copy the capability into every role.
9. Source-managed and user-managed skill bindings must be distinguishable; resync must preserve user-managed bindings.
10. Missing/incompatible required capabilities block affected roles while preserving the last known good applied snapshot.
11. Follow package boundaries and parse all new frontend API responses with zod plus parseWithFallback.
12. When CLI/API behavior changes, update built-in Multica skills and source-map references in the same PR.

Execution method:
- Implement in milestone order, beginning with contract fixtures and daemon scan/preview, while preserving the full schema and end-state architecture from this document. Do not substitute an MVP data model that must later be discarded.
- Before coding, write a short file-level plan and identify the existing helper each new component will reuse.
- Add migrations and sqlc queries deliberately; run make sqlc after query changes.
- Add Go tests for path security, exact `.env` scoping, preview redaction, secret apply authentication/replay protection, at-rest handling, schemas, hashing, protocol auth, malformed manifests, size limits, and offline daemon behavior.
- Add core/view tests for API parsing and preview states.
- Watcher support may land after canonical scanning, but the completed feature must include scheduled and watch-assisted detection; watcher events never replace full rescans.
- Do not preserve a second legacy implementation when replacing an internal flow unless a real product compatibility boundary requires it.
- Preserve unrelated dirty-worktree changes.

Milestone 1 completion means:
- source configuration CRUD exists;
- daemon scan request/result exists;
- the Go scanner recognizes current AgentWaker capabilities and roles;
- server stores immutable scoped snapshots;
- UI can configure, scan, and preview diagnostics and counts;
- no agent, skill, capability, MCP, or env mutation happens yet;
- narrow tests, make test, pnpm typecheck, and relevant pnpm tests pass.

Do not present Milestone 1 as completion of the user goal. Continue through atomic apply, encrypted env synchronization, runtime capability/artifact injection, continuous updates, dependency re-resolution, and rollback.

At handoff, report:
- files changed;
- migrations and protocol changes;
- exact tests run and results;
- remaining phases and risks;
- proof that only the exact role-scoped `.env` body enters source files, parsed values reach the target agent's encrypted configuration and runtime, and unrelated dotenv files never enter results, logs, events, or APIs.
```

## Explicit Non-Goals

- executing AgentWaker validation Ruby scripts in production;
- continuous filesystem watching;
- exposing live credentials outside the dedicated apply, encrypted storage, audited reveal, and task-injection paths;
- automatically deleting agents or skills;
- publishing capability packages outside the workspace;
- treating capability adapters as permission to operate external accounts;
- replacing role-owned Skill wrappers with shared capability entrypoints.
