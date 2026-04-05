CREATE TABLE image_pull_secret_attachments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    image_pull_secret_id UUID NOT NULL,
    agent_id UUID REFERENCES agents(id),
    mcp_id UUID REFERENCES mcps(id),
    hook_id UUID REFERENCES hooks(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK ((agent_id IS NOT NULL)::int + (mcp_id IS NOT NULL)::int + (hook_id IS NOT NULL)::int = 1)
);

CREATE UNIQUE INDEX image_pull_secret_attachments_unique_agent
    ON image_pull_secret_attachments (image_pull_secret_id, agent_id) WHERE agent_id IS NOT NULL;
CREATE UNIQUE INDEX image_pull_secret_attachments_unique_mcp
    ON image_pull_secret_attachments (image_pull_secret_id, mcp_id) WHERE mcp_id IS NOT NULL;
CREATE UNIQUE INDEX image_pull_secret_attachments_unique_hook
    ON image_pull_secret_attachments (image_pull_secret_id, hook_id) WHERE hook_id IS NOT NULL;
