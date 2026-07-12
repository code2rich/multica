-- AgentWaker directory integration (M2): binding ownership, env declarations,
-- and encrypted env values at rest.
--
-- (1) agent_skill.origin: the missing ownership discriminator. Today the
--     wholesale replace path (SetAgentSkills) cannot tell source-managed
--     bindings from user-managed ones, so a resync would clobber user-added
--     skills. The new column lets the apply path remove only origin='source'
--     bindings for an agent while preserving origin='user'.
--
-- (2) agent_env_declaration: the declaration surface recovered from each
--     role's env/.env.example. Carries variable name, required/optional,
--     description, configured boolean, and secret heuristic. The scan preview
--     already exposes this (value-free); apply materializes it here.
--
-- (3) agent.custom_env_encrypted: the synchronized env VALUES, encrypted at
--     rest with the existing application-layer secretbox infrastructure (key
--     MULTICA_AGENT_ENV_SECRET_KEY). The legacy plaintext custom_env JSONB
--     column remains for back-compat reads during migration, but new
--     AgentWaker-synchronized values must NOT be written to it — centralized
--     secret management requires authenticated encryption at rest. Task
--     preparation decrypts only for the owning agent execution.

ALTER TABLE agent_skill ADD COLUMN origin TEXT NOT NULL DEFAULT 'user';
ALTER TABLE agent_skill
    DROP CONSTRAINT IF EXISTS agent_skill_origin_check;
ALTER TABLE agent_skill
    ADD CONSTRAINT agent_skill_origin_check CHECK (origin IN ('user', 'source'));

CREATE TABLE agent_env_declaration (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agent(id) ON DELETE CASCADE,
    source_id UUID REFERENCES agent_source(id) ON DELETE CASCADE,
    source_role_id TEXT,
    var_name TEXT NOT NULL,
    required BOOLEAN NOT NULL DEFAULT FALSE,
    description TEXT NOT NULL DEFAULT '',
    configured BOOLEAN NOT NULL DEFAULT FALSE,
    secret BOOLEAN NOT NULL DEFAULT FALSE,
    source_managed BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (agent_id, var_name)
);

CREATE INDEX idx_agent_env_declaration_agent ON agent_env_declaration (agent_id);
CREATE INDEX idx_agent_env_declaration_source ON agent_env_declaration (source_id);

ALTER TABLE agent ADD COLUMN custom_env_encrypted BYTEA;
