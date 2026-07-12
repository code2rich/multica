-- AgentWaker directory integration: a configured source directory.
--
-- One row per workspace-configured AgentWaker root, owned by a specific daemon
-- runtime (the machine that can read the absolute path). The directory remains
-- the source of truth; Multica stores imported copies and source identities and
-- never turns generated DB state into a second editable source tree.
--
-- canonical_path_hash lets us compare directories without exposing the full
-- absolute path in every API response, and enforces one source per (workspace,
-- canonical directory) so the same root cannot be configured twice.
CREATE TABLE agent_source (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    kind TEXT NOT NULL DEFAULT 'agentwaker_directory',
    daemon_runtime_id UUID NOT NULL REFERENCES agent_runtime(id) ON DELETE CASCADE,
    local_path TEXT NOT NULL,
    canonical_path_hash TEXT NOT NULL,
    sync_mode TEXT NOT NULL DEFAULT 'manual',
    status TEXT NOT NULL DEFAULT 'pending',
    last_snapshot_hash TEXT,
    last_scanned_at TIMESTAMPTZ,
    last_applied_at TIMESTAMPTZ,
    created_by UUID REFERENCES "user"(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, canonical_path_hash),
    CONSTRAINT agent_source_kind_check CHECK (kind IN ('agentwaker_directory')),
    CONSTRAINT agent_source_sync_mode_check CHECK (sync_mode IN ('manual', 'scheduled', 'watch-assisted')),
    CONSTRAINT agent_source_status_check CHECK (
        status IN ('pending', 'scanning', 'ready', 'applying', 'partial', 'failed', 'offline')
    )
);

CREATE INDEX idx_agent_source_workspace ON agent_source (workspace_id);
CREATE INDEX idx_agent_source_daemon_runtime ON agent_source (daemon_runtime_id);
