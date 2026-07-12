-- name: CreateSharedCapability :one
INSERT INTO shared_capability (
    workspace_id, source_id, source_key, name, version, description,
    content_hash, manifest, active_version_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, sqlc.narg('active_version_id')
)
RETURNING *;

-- name: GetSharedCapability :one
SELECT * FROM shared_capability WHERE id = $1;

-- name: GetSharedCapabilityByIdentity :one
-- Stable identity lookup by (workspace, source, source_key). Used by apply to
-- find-or-create on resync; renames do not duplicate because source_key is stable.
SELECT * FROM shared_capability
WHERE workspace_id = $1 AND source_id = $2 AND source_key = $3;

-- name: ListSharedCapabilitiesByWorkspace :many
SELECT * FROM shared_capability
WHERE workspace_id = $1
ORDER BY source_key ASC;

-- name: ListSharedCapabilitiesBySource :many
SELECT * FROM shared_capability
WHERE source_id = $1
ORDER BY source_key ASC;

-- name: UpdateSharedCapabilityActiveVersion :one
-- Points the stable identity at a new immutable version row. The prior version
-- row remains for rollback / historical task evidence.
UPDATE shared_capability SET
    active_version_id = $2,
    version = COALESCE(sqlc.narg('version'), version),
    name = COALESCE(sqlc.narg('name'), name),
    description = COALESCE(sqlc.narg('description'), description),
    content_hash = COALESCE(sqlc.narg('content_hash'), content_hash),
    manifest = COALESCE(sqlc.narg('manifest'), manifest),
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteSharedCapabilitiesBySource :exec
-- Used by detach; cascades to versions and bindings.
DELETE FROM shared_capability WHERE source_id = $1;

-- name: CreateSharedCapabilityVersion :one
INSERT INTO shared_capability_version (
    capability_id, version, content_hash, manifest
) VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetSharedCapabilityVersion :one
SELECT * FROM shared_capability_version WHERE id = $1;

-- name: ListSharedCapabilityVersions :many
SELECT * FROM shared_capability_version
WHERE capability_id = $1
ORDER BY created_at DESC;
