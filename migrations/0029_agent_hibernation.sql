-- How an agent's idle workloads sleep on a runner that can hibernate them:
-- whether they do at all, how long after a turn ends one stays awake first,
-- and how long a sleeping one is kept before the Orchestrator stops it.
--
-- On the class, beside idle_timeout, for the reason that is there: how long an
-- agent may pause describes the kind of agent, and every instance of one wants
-- the same answer.
--
-- Not nullable, and defaulted to the documented values, so an agent that
-- predates the columns hibernates as a new one does. Only a runner advertising
-- the capability hibernates anything, so elsewhere these change nothing.
ALTER TABLE agents
    ADD COLUMN hibernate BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN hibernate_after TEXT NOT NULL DEFAULT '0s',
    ADD COLUMN hibernation_ttl TEXT NOT NULL DEFAULT '168h';
