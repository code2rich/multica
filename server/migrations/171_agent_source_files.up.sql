-- Text source files referenced by imported AgentWaker presentation documents.
-- The scanner excludes binary files, path traversal, and the real env/.env.
ALTER TABLE agent
    ADD COLUMN source_files JSONB NOT NULL DEFAULT '[]'::jsonb;
