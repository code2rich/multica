-- name: CreateAgentCapabilityBinding :one
INSERT INTO agent_capability_binding (
    workspace_id, agent_id, role_skill_id, capability_id, source_id, profile,
    version_requirement, required, permissions, fallback, source_snapshot_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, sqlc.narg('source_snapshot_id')
)
RETURNING *;

-- name: ListAgentCapabilityBindingsByAgent :many
SELECT * FROM agent_capability_binding WHERE agent_id = $1;

-- name: ListAgentCapabilityBindingsByCapability :many
SELECT * FROM agent_capability_binding WHERE capability_id = $1;

-- name: DeleteAgentCapabilityBindingsByAgentSource :exec
-- Remove bindings owned by this (agent, source) so resync re-inserts the
-- current declared set. Bindings from other sources are untouched.
DELETE FROM agent_capability_binding
WHERE agent_id = $1 AND source_id = $2;

-- name: ListAgentCapabilityBindingsBySource :many
SELECT * FROM agent_capability_binding WHERE source_id = $1;
