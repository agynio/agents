CREATE TABLE environments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    organization_id UUID NOT NULL,
    name TEXT NOT NULL,
    flavor_id UUID NOT NULL,
    image TEXT NOT NULL,
    flavor_name TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, name),
    UNIQUE (organization_id, id)
);

CREATE INDEX idx_environments_organization ON environments (organization_id);

CREATE TABLE sandboxes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    organization_id UUID NOT NULL,
    name TEXT NOT NULL,
    environment_id UUID NOT NULL,
    owner_id UUID NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('starting', 'running', 'stopped', 'failed', 'terminated')),
    idle_timeout TEXT NOT NULL,
    ttl TEXT NOT NULL,
    last_session_at TIMESTAMPTZ,
    environment_name TEXT NOT NULL,
    workload_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, name),
    FOREIGN KEY (organization_id, environment_id) REFERENCES environments(organization_id, id)
);

CREATE INDEX idx_sandboxes_organization ON sandboxes (organization_id);
CREATE INDEX idx_sandboxes_owner ON sandboxes (owner_id);
CREATE INDEX idx_sandboxes_environment ON sandboxes (environment_id);
