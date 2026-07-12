-- AgentWaker directory integration (M2) apply-path queries.
-- Kept in a dedicated file so the source-owned binding/origin rules are
-- co-located and easy to audit against the integration plan.

-- name: AddAgentSkillWithOrigin :exec
-- Add (or no-op) a binding carrying an origin so the apply path can record
-- 'source' bindings distinctly from user-managed ('user') ones.
INSERT INTO agent_skill (agent_id, skill_id, origin)
VALUES ($1, $2, $3)
ON CONFLICT (agent_id, skill_id) DO UPDATE SET
    origin = EXCLUDED.origin;

-- name: ListSourceManagedSkillIDsForAgent :many
-- Returns the skill_ids of all source-managed bindings for one agent under a
-- given source. Used by apply to compute the new source-managed set without
-- touching user-managed bindings.
SELECT ask.skill_id
FROM agent_skill ask
JOIN agent_source_skill ass
  ON ass.skill_id = ask.skill_id
WHERE ask.agent_id = $1
  AND ask.origin = 'source'
  AND ass.source_id = $2;

-- name: RemoveAgentSkillIfSourceManaged :exec
-- Remove a binding only if it is source-managed; never removes a user-managed
-- binding. Used when a source deletes a skill the agent no longer references.
DELETE FROM agent_skill
WHERE agent_id = $1 AND skill_id = $2 AND origin = 'source';

-- name: UpsertAgentEnvDeclaration :one
INSERT INTO agent_env_declaration (
    agent_id, source_id, source_role_id, var_name, required, description,
    configured, secret, source_managed
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, TRUE)
ON CONFLICT (agent_id, var_name) DO UPDATE SET
    source_id = EXCLUDED.source_id,
    source_role_id = EXCLUDED.source_role_id,
    required = EXCLUDED.required,
    description = EXCLUDED.description,
    configured = EXCLUDED.configured,
    secret = EXCLUDED.secret,
    source_managed = TRUE,
    updated_at = now()
RETURNING *;

-- name: ListAgentEnvDeclarations :many
SELECT * FROM agent_env_declaration WHERE agent_id = $1 ORDER BY var_name ASC;

-- name: ListAgentEnvDeclarationsBySource :many
SELECT * FROM agent_env_declaration WHERE source_id = $1 ORDER BY var_name ASC;

-- name: DeleteSourceManagedEnvDeclarations :exec
-- Remove declarations owned by this source so apply can re-insert the current
-- declared set. Declarations for keys still managed by the source are
-- re-created by UpsertAgentEnvDeclaration in the same transaction.
DELETE FROM agent_env_declaration WHERE source_id = $1 AND source_managed = TRUE;

-- name: DeleteAgentEnvDeclarationIfSourceManaged :exec
DELETE FROM agent_env_declaration
WHERE agent_id = $1 AND var_name = ANY($2::text[]) AND source_managed = TRUE;

-- name: UpdateAgentEncryptedEnv :exec
-- Stores the secretbox-sealed synchronized env values. The plaintext is never
-- written here; task preparation decrypts only for the owning agent execution.
UPDATE agent SET
    custom_env_encrypted = $2,
    updated_at = now()
WHERE id = $1;

-- name: ClearAgentEncryptedEnvForSource :exec
-- Clears the encrypted env column when the source-managed values are removed
-- (used by source-authoritative merge policy when the source declares no keys,
-- or by detach). User-managed plaintext values in custom_env are not touched
-- by this query — they follow the legacy env endpoint.
UPDATE agent SET
    custom_env_encrypted = NULL,
    updated_at = now()
WHERE id = $1;

-- name: SetAgentOwnerIfNull :exec
-- Backfills imports created before apply propagated the authenticated user.
-- Existing explicit ownership is preserved across all subsequent syncs.
UPDATE agent SET
    owner_id = $2,
    updated_at = now()
WHERE id = $1 AND owner_id IS NULL;

-- name: SetSkillCreatorIfNull :exec
-- Ensures every synchronized skill has an adding user without taking ownership
-- away from an adopted skill that already records a creator.
UPDATE skill SET
    created_by = $2,
    updated_at = now()
WHERE id = $1 AND created_by IS NULL;
