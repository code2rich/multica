-- AgentWaker directory integration (M3): content-addressed capability files.
--
-- Each row stores one immutable file body (entrypoint or supporting file) for a
-- shared capability version, keyed by its sha256 content hash. Multiple
-- capability versions referencing the same file body share one row — this is
-- the single-instance, content-addressed store that avoids copying the
-- capability into every role-owned skill.
--
-- Runtime materialization (task preparation) reads these rows to write the
-- capability entrypoint + supporting files into the execution sandbox exactly
-- once per task. The body is public text (never env/secret data); the secret
-- surface remains the agent.custom_env_encrypted column from migration 168.

CREATE TABLE shared_capability_file (
    sha256 TEXT PRIMARY KEY,
    body TEXT NOT NULL,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Per-version file membership: which files belong to which capability version,
-- and the relative path at which the runtime must install each one.
CREATE TABLE shared_capability_version_file (
    capability_version_id UUID NOT NULL REFERENCES shared_capability_version(id) ON DELETE CASCADE,
    sha256 TEXT NOT NULL REFERENCES shared_capability_file(sha256) ON DELETE CASCADE,
    path TEXT NOT NULL,
    is_entrypoint BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (capability_version_id, path)
);

CREATE INDEX idx_shared_capability_version_file_version
    ON shared_capability_version_file (capability_version_id);

-- Entrypoint content for a capability version (the SKILL.md body). Stored
-- separately so the runtime can fetch it as the bundle Content without scanning
-- the file membership table for is_entrypoint.
-- We use shared_capability_version_file with is_entrypoint=TRUE for the
-- entrypoint, so no extra table is needed; this comment documents the convention.
