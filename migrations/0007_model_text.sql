ALTER TABLE agents
    ALTER COLUMN model TYPE UUID USING model::uuid;
