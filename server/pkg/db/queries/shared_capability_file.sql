-- name: UpsertSharedCapabilityFile :one
-- Content-addressed store: insert-if-absent. Multiple versions referencing the
-- same body share one row. The sha256 is the caller-computed content digest.
INSERT INTO shared_capability_file (sha256, body, size_bytes)
VALUES ($1, $2, $3)
ON CONFLICT (sha256) DO UPDATE SET sha256 = EXCLUDED.sha256
RETURNING *;

-- name: GetSharedCapabilityFile :one
SELECT * FROM shared_capability_file WHERE sha256 = $1;

-- name: UpsertSharedCapabilityVersionFile :exec
-- Record that one file (by content hash) is a member of one capability version
-- at a relative path. is_entrypoint marks the entrypoint body.
INSERT INTO shared_capability_version_file (
    capability_version_id, sha256, path, is_entrypoint
) VALUES ($1, $2, $3, $4)
ON CONFLICT (capability_version_id, path) DO UPDATE SET
    sha256 = EXCLUDED.sha256,
    is_entrypoint = EXCLUDED.is_entrypoint;

-- name: ListSharedCapabilityVersionFiles :many
SELECT scvf.path, scvf.is_entrypoint, scf.body, scf.size_bytes
FROM shared_capability_version_file scvf
JOIN shared_capability_file scf ON scf.sha256 = scvf.sha256
WHERE scvf.capability_version_id = $1
ORDER BY scvf.path ASC;

-- name: ListAgentCapabilityBundlesForAgent :many
-- The runtime-materialization query: for one agent, join each capability binding
-- to its shared capability's ACTIVE version, returning the rows the service
-- layer turns into AgentSkillData bundles. Returns one row per (binding, version
-- file) so the caller can assemble content + files per capability.
SELECT
    sc.id AS capability_id,
    sc.source_key,
    sc.name,
    sc.description,
    sc.version,
    sc.content_hash,
    sc.active_version_id,
    acb.id AS binding_id,
    acb.profile,
    acb.version_requirement,
    acb.required,
    acb.permissions,
    acb.fallback,
    scvf.path AS file_path,
    scvf.is_entrypoint,
    scf.body AS file_body
FROM agent_capability_binding acb
JOIN shared_capability sc ON sc.id = acb.capability_id
LEFT JOIN shared_capability_version_file scvf ON scvf.capability_version_id = sc.active_version_id
LEFT JOIN shared_capability_file scf ON scf.sha256 = scvf.sha256
WHERE acb.agent_id = $1
ORDER BY sc.source_key ASC, scvf.path ASC;
