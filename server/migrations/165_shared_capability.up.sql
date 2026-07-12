-- AgentWaker directory integration (M2): shared capabilities.
--
-- A shared capability is a workspace-installed package (e.g.
-- information-collection, visual-generation) sourced from an AgentWaker
-- directory. It has ONE stable identity per (workspace_id, source_id,
-- source_key) but is continuously versioned: every resync compares identity,
-- semantic version, manifest, profiles, adapters, permissions, and content
-- hash, and a compatible update creates a new immutable version row while
-- preserving the stable identity. Consuming roles bind to the stable identity;
-- they never get a private editable copy.
--
-- Do NOT model a shared capability as an ordinary role skill. It carries
-- profiles, version constraints, permissions, and many-to-many consumers that
-- agent_skill cannot represent.

CREATE TABLE shared_capability (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    source_id UUID NOT NULL REFERENCES agent_source(id) ON DELETE CASCADE,
    source_key TEXT NOT NULL,
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    content_hash TEXT NOT NULL,
    manifest JSONB NOT NULL DEFAULT '{}'::jsonb,
    active_version_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- One stable identity per (workspace, source, capability id).
    UNIQUE (workspace_id, source_id, source_key)
);

CREATE INDEX idx_shared_capability_workspace ON shared_capability (workspace_id);
CREATE INDEX idx_shared_capability_source ON shared_capability (source_id);

-- Immutable per-version snapshot of a capability package. A resync that changes
-- content (even at the same version) or bumps the version creates a new row.
-- Old rows are retained for rollback and historical task evidence; only
-- shared_capability.active_version_id moves.
CREATE TABLE shared_capability_version (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    capability_id UUID NOT NULL REFERENCES shared_capability(id) ON DELETE CASCADE,
    version TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    manifest JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE shared_capability
    ADD CONSTRAINT shared_capability_active_version_fkey
    FOREIGN KEY (active_version_id) REFERENCES shared_capability_version(id) ON DELETE SET NULL;

CREATE INDEX idx_shared_capability_version_capability
    ON shared_capability_version (capability_id);
