ALTER TABLE agents
    ADD COLUMN capabilities JSONB NOT NULL DEFAULT '[]'::jsonb;
