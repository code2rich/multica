DROP TABLE IF EXISTS agent_env_declaration;

ALTER TABLE agent DROP COLUMN IF EXISTS custom_env_encrypted;

ALTER TABLE agent_skill DROP CONSTRAINT IF EXISTS agent_skill_origin_check;
ALTER TABLE agent_skill DROP COLUMN IF EXISTS origin;
