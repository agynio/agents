CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

DROP TABLE IF EXISTS attachments;
DROP TABLE IF EXISTS variables;
DROP TABLE IF EXISTS memory_buckets;
DROP TABLE IF EXISTS workspace_configurations;
DROP TABLE IF EXISTS mcp_servers;
DROP TABLE IF EXISTS tools;
DROP TABLE IF EXISTS agents;

CREATE TABLE agents (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT '',
    model UUID NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    configuration TEXT NOT NULL DEFAULT '',
    image TEXT NOT NULL DEFAULT '',
    resources_requests_cpu TEXT NOT NULL DEFAULT '',
    resources_requests_memory TEXT NOT NULL DEFAULT '',
    resources_limits_cpu TEXT NOT NULL DEFAULT '',
    resources_limits_memory TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE volumes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    persistent BOOLEAN NOT NULL DEFAULT FALSE,
    mount_path TEXT NOT NULL DEFAULT '',
    size TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE mcps (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_id UUID NOT NULL REFERENCES agents(id),
    image TEXT NOT NULL DEFAULT '',
    command TEXT NOT NULL DEFAULT '',
    resources_requests_cpu TEXT NOT NULL DEFAULT '',
    resources_requests_memory TEXT NOT NULL DEFAULT '',
    resources_limits_cpu TEXT NOT NULL DEFAULT '',
    resources_limits_memory TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE skills (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_id UUID NOT NULL REFERENCES agents(id),
    name TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE hooks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_id UUID NOT NULL REFERENCES agents(id),
    event TEXT NOT NULL DEFAULT '',
    "function" TEXT NOT NULL DEFAULT '',
    image TEXT NOT NULL DEFAULT '',
    resources_requests_cpu TEXT NOT NULL DEFAULT '',
    resources_requests_memory TEXT NOT NULL DEFAULT '',
    resources_limits_cpu TEXT NOT NULL DEFAULT '',
    resources_limits_memory TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE volume_attachments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    volume_id UUID NOT NULL REFERENCES volumes(id),
    agent_id UUID REFERENCES agents(id),
    mcp_id UUID REFERENCES mcps(id),
    hook_id UUID REFERENCES hooks(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK ((agent_id IS NOT NULL)::int + (mcp_id IS NOT NULL)::int + (hook_id IS NOT NULL)::int = 1)
);

CREATE TABLE envs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    agent_id UUID REFERENCES agents(id),
    mcp_id UUID REFERENCES mcps(id),
    hook_id UUID REFERENCES hooks(id),
    value TEXT,
    secret_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK ((agent_id IS NOT NULL)::int + (mcp_id IS NOT NULL)::int + (hook_id IS NOT NULL)::int = 1),
    CHECK ((value IS NOT NULL)::int + (secret_id IS NOT NULL)::int = 1)
);

CREATE TABLE init_scripts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    script TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    agent_id UUID REFERENCES agents(id),
    mcp_id UUID REFERENCES mcps(id),
    hook_id UUID REFERENCES hooks(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK ((agent_id IS NOT NULL)::int + (mcp_id IS NOT NULL)::int + (hook_id IS NOT NULL)::int = 1)
);

CREATE UNIQUE INDEX volume_attachments_unique_agent ON volume_attachments (volume_id, agent_id) WHERE agent_id IS NOT NULL;
CREATE UNIQUE INDEX volume_attachments_unique_mcp ON volume_attachments (volume_id, mcp_id) WHERE mcp_id IS NOT NULL;
CREATE UNIQUE INDEX volume_attachments_unique_hook ON volume_attachments (volume_id, hook_id) WHERE hook_id IS NOT NULL;
