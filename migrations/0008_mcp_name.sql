ALTER TABLE mcps
    ADD COLUMN name TEXT NOT NULL DEFAULT '';

ALTER TABLE mcps
    ADD CONSTRAINT mcps_agent_name_unique UNIQUE (agent_id, name);
