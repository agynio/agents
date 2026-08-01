-- How long an instance may sit idle before the platform pauses it.
--
-- On the class, not the instance: idleness is measured per instance, but how
-- much of it is tolerable describes the kind of agent. A long-lived assistant
-- and a one-shot worker want different answers, and every instance of one
-- agent wants the same one.
--
-- Nullable, and null means never. An agent that predates this column keeps
-- instances until something else stops them, which is what it did yesterday.
ALTER TABLE agents
    ADD COLUMN instance_idle_ttl TEXT;

-- The GC sweeps active instances by last_activity_at. Without this it reads
-- every active instance in the deployment on every tick.
CREATE INDEX IF NOT EXISTS agent_instances_active_last_activity_idx
    ON agent_instances (last_activity_at)
    WHERE state = 'active';
