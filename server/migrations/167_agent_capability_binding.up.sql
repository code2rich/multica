-- AgentWaker directory integration (M2): capability bindings.
--
-- One row per (role-owned skill, shared capability) consumer relationship, as
-- declared by the role's capabilities.yaml. Effective permissions are the
-- intersection of system policy, capability support, role declaration, and
-- current user approval; import rejects attempted expansion (the plan flags
-- it as a diagnostic, not a silent clamp).
--
-- source_snapshot_id records which scan produced this binding, so a rollback
-- or diff can attribute every binding to its evidence.

CREATE TABLE agent_capability_binding (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agent(id) ON DELETE CASCADE,
    role_skill_id UUID NOT NULL REFERENCES skill(id) ON DELETE CASCADE,
    capability_id UUID NOT NULL REFERENCES shared_capability(id) ON DELETE CASCADE,
    source_id UUID NOT NULL REFERENCES agent_source(id) ON DELETE CASCADE,
    profile TEXT NOT NULL,
    version_requirement TEXT NOT NULL,
    required BOOLEAN NOT NULL DEFAULT FALSE,
    permissions JSONB NOT NULL DEFAULT '{}'::jsonb,
    fallback JSONB NOT NULL DEFAULT '{}'::jsonb,
    source_snapshot_id UUID REFERENCES agent_source_snapshot(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- One binding per (role skill, capability) per source.
    UNIQUE (source_id, role_skill_id, capability_id)
);

CREATE INDEX idx_agent_capability_binding_agent ON agent_capability_binding (agent_id);
CREATE INDEX idx_agent_capability_binding_capability ON agent_capability_binding (capability_id);
CREATE INDEX idx_agent_capability_binding_source ON agent_capability_binding (source_id);
