-- name: CreateAgentSource :one
INSERT INTO agent_source (
    workspace_id, kind, daemon_runtime_id, local_path, canonical_path_hash,
    sync_mode, status, created_by
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, sqlc.narg('created_by')
)
RETURNING *;

-- name: GetAgentSource :one
SELECT * FROM agent_source
WHERE id = $1;

-- name: GetAgentSourceInWorkspace :one
SELECT * FROM agent_source
WHERE id = $1 AND workspace_id = $2;

-- name: ListAgentSourcesByWorkspace :many
SELECT * FROM agent_source
WHERE workspace_id = $1
ORDER BY created_at ASC;

-- name: UpdateAgentSource :one
-- Partial update; each field uses COALESCE so omitted fields are preserved.
-- status transitions are handled by UpdateAgentSourceStatus to keep them
-- explicit and auditable.
UPDATE agent_source SET
    daemon_runtime_id = COALESCE(sqlc.narg('daemon_runtime_id'), daemon_runtime_id),
    local_path = COALESCE(sqlc.narg('local_path'), local_path),
    canonical_path_hash = COALESCE(sqlc.narg('canonical_path_hash'), canonical_path_hash),
    sync_mode = COALESCE(sqlc.narg('sync_mode'), sync_mode),
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateAgentSourceStatus :one
-- Records a status transition and stamps the relevant audit timestamp.
-- last_snapshot_hash / last_scanned_at are set together on a successful scan;
-- last_applied_at is set on a successful apply.
UPDATE agent_source SET
    status = $2,
    last_snapshot_hash = COALESCE(sqlc.narg('last_snapshot_hash'), last_snapshot_hash),
    last_scanned_at = COALESCE(sqlc.narg('last_scanned_at'), last_scanned_at),
    last_applied_at = COALESCE(sqlc.narg('last_applied_at'), last_applied_at),
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteAgentSource :exec
DELETE FROM agent_source
WHERE id = $1;

-- name: CreateAgentSourceSnapshot :one
INSERT INTO agent_source_snapshot (
    source_id, directory_hash, schema_versions, manifest, status, diagnostics,
    lock_yaml, scanner_version
) VALUES (
    $1, $2, $3, $4, $5, $6, sqlc.narg('lock_yaml'), sqlc.narg('scanner_version')
)
RETURNING *;

-- name: GetAgentSourceSnapshot :one
SELECT * FROM agent_source_snapshot
WHERE id = $1;

-- name: GetAgentSourceSnapshotInSource :one
SELECT * FROM agent_source_snapshot
WHERE id = $1 AND source_id = $2;

-- name: ListAgentSourceSnapshots :many
SELECT * FROM agent_source_snapshot
WHERE source_id = $1
ORDER BY created_at DESC;

-- name: GetLatestAppliedAgentSourceSnapshot :one
-- The currently-active applied snapshot for a source, or NULL if none has been
-- applied yet. Used for last-known-good behavior and the diff/plan endpoint.
SELECT * FROM agent_source_snapshot
WHERE source_id = $1 AND status = 'applied'
ORDER BY applied_at DESC NULLS LAST
LIMIT 1;

-- name: MarkAgentSourceSnapshotSuperseded :exec
-- Called when a newer snapshot is applied. The previously-applied row becomes
-- 'superseded' so the history is retained for rollback evidence.
UPDATE agent_source_snapshot SET
    status = 'superseded'
WHERE source_id = $1 AND id != $2 AND status = 'applied';

-- name: MarkAgentSourceSnapshotApplied :one
-- Flips a preview snapshot to applied after the apply transaction commits,
-- stamps applied_at, and stores the resolved lock YAML.
UPDATE agent_source_snapshot SET
    status = 'applied',
    applied_at = now(),
    lock_yaml = sqlc.narg('lock_yaml')
WHERE id = $1 AND status IN ('preview', 'failed')
RETURNING *;

-- name: MarkAgentSourceSnapshotFailed :exec
UPDATE agent_source_snapshot SET
    status = 'failed'
WHERE id = $1;

-- name: ListSchedulableAgentSources :many
-- Returns sources that are configured for periodic scanning and are in a state
-- that permits a rescan (ready or failed — not pending/scanning/applying).
SELECT * FROM agent_source
WHERE sync_mode = 'scheduled'
  AND status IN ('ready', 'failed')
ORDER BY last_scanned_at ASC NULLS FIRST;
