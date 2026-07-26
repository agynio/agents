-- Placement moves from a flavor id to a runner plus a flavor name: the runner
-- declares its own catalog, and the name is resolved against it at workload
-- start rather than being a foreign key to a platform record.
ALTER TABLE environments
    ADD COLUMN IF NOT EXISTS runner_id UUID,
    ADD COLUMN IF NOT EXISTS flavor TEXT NOT NULL DEFAULT '';

-- flavor_id is retained for callers still reading it and is no longer required.
-- New environments name a runner instead; a null runner_id is an environment
-- written before this change, which stays unschedulable until it is updated.
ALTER TABLE environments
    ALTER COLUMN flavor_id DROP NOT NULL;

CREATE INDEX IF NOT EXISTS idx_environments_runner ON environments (runner_id);
