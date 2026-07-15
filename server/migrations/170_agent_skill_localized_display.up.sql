-- Localized presentation fields for AgentWaker imports.
-- Runtime execution continues to use agent.instructions and skill.content.
ALTER TABLE agent
    ADD COLUMN instructions_zh TEXT NOT NULL DEFAULT '';

ALTER TABLE skill
    ADD COLUMN description_zh TEXT NOT NULL DEFAULT '';
