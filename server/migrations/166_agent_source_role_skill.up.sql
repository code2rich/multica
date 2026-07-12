-- AgentWaker directory integration (M2): source identity mappings.
--
-- These tables are the stable-identity layer that survives display-name
-- changes. Source identity (the AgentWaker role/skill id) wins over same-name
-- matching: a rename updates the mapped Multica object instead of duplicating
-- it, and a same-name unrelated workspace object is a conflict requiring an
-- explicit action (handled by the plan generator).

-- Maps one AgentWaker role (source identity) to one Multica agent. Survives
-- display-name changes. last_import_hash drives no-op detection on resync.
CREATE TABLE agent_source_role (
    source_id UUID NOT NULL REFERENCES agent_source(id) ON DELETE CASCADE,
    source_role_id TEXT NOT NULL,
    agent_id UUID NOT NULL REFERENCES agent(id) ON DELETE CASCADE,
    last_import_hash TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (source_id, source_role_id)
);

CREATE INDEX idx_agent_source_role_agent ON agent_source_role (agent_id);

-- Maps one AgentWaker role-owned skill (source identity) to one Multica skill.
-- is_meta distinguishes the role's meta routing skill from its specialists.
-- content_hash drives no-op detection. The skill.config.origin JSONB carries
-- display provenance, but THIS row is the relational source of truth.
CREATE TABLE agent_source_skill (
    source_id UUID NOT NULL REFERENCES agent_source(id) ON DELETE CASCADE,
    source_role_id TEXT NOT NULL,
    source_skill_id TEXT NOT NULL,
    skill_id UUID NOT NULL REFERENCES skill(id) ON DELETE CASCADE,
    is_meta BOOLEAN NOT NULL DEFAULT FALSE,
    content_hash TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (source_id, source_role_id, source_skill_id)
);

CREATE INDEX idx_agent_source_skill_skill ON agent_source_skill (skill_id);
