CREATE TABLE agent_source_automation (
    source_id UUID NOT NULL,
    source_role_id TEXT NOT NULL,
    source_automation_id TEXT NOT NULL,
    autopilot_id UUID NOT NULL,
    trigger_id UUID NOT NULL,
    last_import_hash TEXT NOT NULL,
    last_snapshot_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
