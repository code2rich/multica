-- AgentWaker directory integration: immutable scan/apply snapshots.
--
-- Each row is one sanitized scan or apply record. The manifest column carries
-- the value-free, sanitized parsed manifest (env key names, configured booleans,
-- and value digests — NEVER raw .env values). Diagnostics holds validation
-- errors/warnings. lock_yaml holds the Multica-side resolved lock written only
-- after a successful apply (see the integration plan's "Lock and Reproducibility"
-- section).
--
-- Snapshots are append-only: a new scan creates a new 'preview' row; an apply
-- flips one row to 'applied' and the previously-applied row to 'superseded'.
-- Last-known-good behavior keeps the most recent 'applied' row active when a
-- later scan/apply fails.
CREATE TABLE agent_source_snapshot (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id UUID NOT NULL REFERENCES agent_source(id) ON DELETE CASCADE,
    directory_hash TEXT NOT NULL,
    schema_versions JSONB NOT NULL DEFAULT '{}'::jsonb,
    manifest JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'preview',
    diagnostics JSONB NOT NULL DEFAULT '[]'::jsonb,
    lock_yaml TEXT,
    scanner_version TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    applied_at TIMESTAMPTZ,
    CONSTRAINT agent_source_snapshot_status_check CHECK (
        status IN ('preview', 'applied', 'failed', 'superseded')
    )
);

CREATE INDEX idx_agent_source_snapshot_source_status
    ON agent_source_snapshot (source_id, status);
CREATE INDEX idx_agent_source_snapshot_source_created
    ON agent_source_snapshot (source_id, created_at DESC);
