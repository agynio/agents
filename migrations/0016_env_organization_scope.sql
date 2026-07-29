-- An env names a value or a secret, and nothing on the row said which
-- organization owned it. Reads stayed inside one tenant only by being narrowed
-- to a parent id, which is why an unfiltered list had to be refused outright
-- rather than authorized. The organization is already derivable from the parent
-- chain, so it is recorded on the row and reads are scoped by it instead.
--
-- environment_id joins agent_id, mcp_id and hook_id as a target. An environment
-- belongs to an organization rather than to an agent, which is the other reason
-- the row now has to carry the organization itself.
ALTER TABLE envs
    ADD COLUMN organization_id UUID,
    ADD COLUMN environment_id UUID;

UPDATE envs
SET organization_id = agents.organization_id
FROM agents
WHERE envs.agent_id = agents.id;

UPDATE envs
SET organization_id = agents.organization_id
FROM mcps
    JOIN agents ON agents.id = mcps.agent_id
WHERE envs.mcp_id = mcps.id;

UPDATE envs
SET organization_id = agents.organization_id
FROM hooks
    JOIN agents ON agents.id = hooks.agent_id
WHERE envs.hook_id = hooks.id;

-- Every row must resolve: exactly one target is set, each target is a foreign
-- key, and mcps and hooks both carry a non-null agent. A row that does not
-- resolve is corruption no automatic choice can repair. Deleting it would drop
-- a secret reference some workload still reads, and parking it in a placeholder
-- organization would hand it to whoever holds that organization -- the opposite
-- of what scoping these rows is for. Stop instead: migrations apply in one
-- transaction, so the database is left exactly as it was.
DO $$
DECLARE
    unresolved BIGINT;
BEGIN
    SELECT count(*) INTO unresolved FROM envs WHERE organization_id IS NULL;
    IF unresolved > 0 THEN
        RAISE EXCEPTION 'envs: % row(s) reach no organization through agent_id, mcp_id or hook_id', unresolved;
    END IF;
END
$$;

ALTER TABLE envs
    ALTER COLUMN organization_id SET NOT NULL;

-- The unnamed target check from 0003, which admits only an agent, an mcp or a
-- hook.
ALTER TABLE envs
    DROP CONSTRAINT envs_check;

ALTER TABLE envs
    ADD CONSTRAINT envs_target_check CHECK (
        (agent_id IS NOT NULL)::int + (mcp_id IS NOT NULL)::int + (hook_id IS NOT NULL)::int + (environment_id IS NOT NULL)::int = 1
    ),
    -- Composite, as sandboxes reference environments, so an env and the
    -- environment it targets cannot drift into different organizations.
    ADD CONSTRAINT envs_environment_fkey FOREIGN KEY (organization_id, environment_id) REFERENCES environments (organization_id, id);

CREATE INDEX envs_organization_id_idx ON envs (organization_id);
CREATE INDEX envs_environment_id_idx ON envs (environment_id);
