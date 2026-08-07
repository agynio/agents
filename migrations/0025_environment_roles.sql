-- Environments carry the same two-layer model agents do: any member may author
-- one, and who else may read, edit or run in it is a per-environment grant.
-- `user` is the role agents have no equivalent of -- it opens an interactive
-- shell onto the environment without any configuration access.
CREATE TABLE environment_roles (
    environment_id UUID NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    identity_id UUID NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('owner', 'maintainer', 'user')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (environment_id, identity_id)
);

CREATE INDEX idx_environment_roles_identity ON environment_roles (identity_id);
