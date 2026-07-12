-- name: UpsertAgentSourceRole :one
-- find-or-create the stable (source, role) -> agent mapping. ON CONFLICT
-- updates last_import_hash so no-op rescans are detectable.
INSERT INTO agent_source_role (source_id, source_role_id, agent_id, last_import_hash)
VALUES ($1, $2, $3, sqlc.narg('last_import_hash'))
ON CONFLICT (source_id, source_role_id) DO UPDATE SET
    agent_id = EXCLUDED.agent_id,
    last_import_hash = COALESCE(EXCLUDED.last_import_hash, agent_source_role.last_import_hash),
    updated_at = now()
RETURNING *;

-- name: GetAgentSourceRole :one
SELECT * FROM agent_source_role
WHERE source_id = $1 AND source_role_id = $2;

-- name: ListAgentSourceRolesBySource :many
SELECT * FROM agent_source_role WHERE source_id = $1;

-- name: UpdateAgentSourceRoleHash :exec
UPDATE agent_source_role SET
    last_import_hash = $3,
    updated_at = now()
WHERE source_id = $1 AND source_role_id = $2;

-- name: UpsertAgentSourceSkill :one
-- find-or-create the stable (source, role, skill) -> skill mapping.
INSERT INTO agent_source_skill (
    source_id, source_role_id, source_skill_id, skill_id, is_meta, content_hash
) VALUES ($1, $2, $3, $4, $5, sqlc.narg('content_hash'))
ON CONFLICT (source_id, source_role_id, source_skill_id) DO UPDATE SET
    skill_id = EXCLUDED.skill_id,
    is_meta = EXCLUDED.is_meta,
    content_hash = COALESCE(EXCLUDED.content_hash, agent_source_skill.content_hash),
    updated_at = now()
RETURNING *;

-- name: GetAgentSourceSkill :one
SELECT * FROM agent_source_skill
WHERE source_id = $1 AND source_role_id = $2 AND source_skill_id = $3;

-- name: ListAgentSourceSkillsBySourceRole :many
SELECT * FROM agent_source_skill
WHERE source_id = $1 AND source_role_id = $2;

-- name: ListAgentSourceSkillsBySource :many
SELECT * FROM agent_source_skill WHERE source_id = $1;

-- name: DeleteAgentSourceBindingsForAgent :exec
-- Remove ONLY source-managed agent_skill rows for one agent; user-managed
-- (origin='user') bindings are preserved. This is the core of the
-- "resync preserves user-managed bindings" rule.
DELETE FROM agent_skill
WHERE agent_id = $1
  AND origin = 'source'
  AND skill_id IN (
      SELECT skill_id FROM agent_source_skill WHERE source_id = $2
  );
