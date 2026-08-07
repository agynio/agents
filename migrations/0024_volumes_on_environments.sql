-- A volume described nothing that existed until something mounted it: a
-- free-standing organization row holding a mount path and a size, reaching a
-- workload only through a separate attachment. It becomes a sub-resource of the
-- thing that mounts it.
--
-- Nothing is translated. A definition carried no name and no target, so there
-- is no answer to which environment an existing row belonged to that is not a
-- guess. Operators redeclare storage on the environments that need it.
DROP TABLE IF EXISTS volume_attachments;
DROP TABLE IF EXISTS volumes;

CREATE TABLE volumes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    organization_id UUID NOT NULL,
    environment_id UUID,
    mcp_id UUID REFERENCES mcps(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    mount_path TEXT NOT NULL,
    persistent BOOLEAN NOT NULL DEFAULT FALSE,
    size TEXT,
    storage_class TEXT,
    ttl TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT volumes_target_check CHECK (
        (environment_id IS NOT NULL)::int + (mcp_id IS NOT NULL)::int = 1
    ),
    -- size and persistence are biconditional, which is what lets the CLI make
    -- --size the whole control: given, the volume is a disk; omitted, scratch.
    CONSTRAINT volumes_size_check CHECK (
        persistent = (size IS NOT NULL AND size <> '')
    ),
    -- Composite, as sandboxes reference environments, so a volume and the
    -- environment it targets cannot drift into different organizations.
    CONSTRAINT volumes_environment_fkey FOREIGN KEY (organization_id, environment_id)
        REFERENCES environments (organization_id, id) ON DELETE CASCADE
);

-- Names are the contract MCPs reference, and paths are what a container sees;
-- both are unique within one target and deliberately reusable across targets.
CREATE UNIQUE INDEX volumes_environment_name_idx ON volumes (environment_id, name) WHERE environment_id IS NOT NULL;
CREATE UNIQUE INDEX volumes_environment_path_idx ON volumes (environment_id, mount_path) WHERE environment_id IS NOT NULL;
CREATE UNIQUE INDEX volumes_mcp_name_idx ON volumes (mcp_id, name) WHERE mcp_id IS NOT NULL;
CREATE UNIQUE INDEX volumes_mcp_path_idx ON volumes (mcp_id, mount_path) WHERE mcp_id IS NOT NULL;
CREATE INDEX volumes_organization_id_idx ON volumes (organization_id);

-- An MCP belonged to an agent and carried no organization of its own, so an
-- environment could not define one and a row could not be scoped without
-- walking to its agent.
ALTER TABLE mcps
    ADD COLUMN organization_id UUID,
    ADD COLUMN environment_id UUID,
    ADD COLUMN shared_volumes TEXT[] NOT NULL DEFAULT '{}';

UPDATE mcps
SET organization_id = agents.organization_id
FROM agents
WHERE mcps.agent_id = agents.id;

-- Every row must resolve. A row that does not is corruption no automatic choice
-- can repair, and migrations apply in one transaction, so stopping leaves the
-- database exactly as it was.
DO $$
DECLARE
    unresolved BIGINT;
BEGIN
    SELECT count(*) INTO unresolved FROM mcps WHERE organization_id IS NULL;
    IF unresolved > 0 THEN
        RAISE EXCEPTION 'mcps: % row(s) reach no organization through agent_id', unresolved;
    END IF;
END
$$;

ALTER TABLE mcps
    ALTER COLUMN organization_id SET NOT NULL,
    ALTER COLUMN agent_id DROP NOT NULL,
    ADD CONSTRAINT mcps_target_check CHECK (
        (agent_id IS NOT NULL)::int + (environment_id IS NOT NULL)::int = 1
    ),
    ADD CONSTRAINT mcps_environment_fkey FOREIGN KEY (organization_id, environment_id)
        REFERENCES environments (organization_id, id) ON DELETE CASCADE;

CREATE INDEX mcps_organization_id_idx ON mcps (organization_id);
CREATE INDEX mcps_environment_id_idx ON mcps (environment_id);
CREATE UNIQUE INDEX mcps_environment_name_idx ON mcps (environment_id, name) WHERE environment_id IS NOT NULL;

-- An init script joins ENV in accepting an environment: what a workload runs
-- before its agent CLI is a property of the environment, not only of the agent.
ALTER TABLE init_scripts
    ADD COLUMN organization_id UUID,
    ADD COLUMN environment_id UUID,
    ADD COLUMN name TEXT NOT NULL DEFAULT '';

UPDATE init_scripts
SET organization_id = agents.organization_id
FROM agents
WHERE init_scripts.agent_id = agents.id;

UPDATE init_scripts
SET organization_id = mcps.organization_id
FROM mcps
WHERE init_scripts.mcp_id = mcps.id;

DO $$
DECLARE
    unresolved BIGINT;
BEGIN
    SELECT count(*) INTO unresolved FROM init_scripts WHERE organization_id IS NULL;
    IF unresolved > 0 THEN
        RAISE EXCEPTION 'init_scripts: % row(s) reach no organization through agent_id or mcp_id', unresolved;
    END IF;
END
$$;

ALTER TABLE init_scripts
    ALTER COLUMN organization_id SET NOT NULL,
    DROP CONSTRAINT IF EXISTS init_scripts_check,
    ADD CONSTRAINT init_scripts_target_check CHECK (
        (agent_id IS NOT NULL)::int + (mcp_id IS NOT NULL)::int + (environment_id IS NOT NULL)::int = 1
    ),
    ADD CONSTRAINT init_scripts_environment_fkey FOREIGN KEY (organization_id, environment_id)
        REFERENCES environments (organization_id, id) ON DELETE CASCADE;

CREATE INDEX init_scripts_organization_id_idx ON init_scripts (organization_id);
CREATE INDEX init_scripts_environment_id_idx ON init_scripts (environment_id);

-- Running in an environment reaches its secrets, egress credentials and volume
-- contents, so it is a grant of its own rather than a consequence of being able
-- to see the environment. Existing environments stay reachable by any member.
ALTER TABLE environments
    ADD COLUMN availability TEXT NOT NULL DEFAULT 'internal'
        CHECK (availability IN ('internal', 'private'));
