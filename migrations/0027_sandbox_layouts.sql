-- One identity's set of open shells in one sandbox -- what a client reopens to
-- find its work where it left it.
--
-- Keyed by identity as well as sandbox: a collaborator opening a shared
-- sandbox arrives at their own tabs rather than joining the owner's. That is a
-- presentation boundary and not a security one, since every shell in a
-- container is visible from inside it to anyone who can open one there.
--
-- Tabs are a document rather than a table of rows. Reordering, closing and
-- opening are all one write, and the fields this is expected to grow -- split
-- geometry, sizes, which tab had focus -- arrive without a schema change each.
-- `version` is what makes two devices safe: a writer supplies the version it
-- read, and the loser of a race refetches instead of silently overwriting.
--
-- Deleted with the sandbox. A sandbox that merely stopped keeps its layouts,
-- which is the entire reason they live out here rather than in the container.
CREATE TABLE sandbox_layouts (
    sandbox_id UUID NOT NULL REFERENCES sandboxes(id) ON DELETE CASCADE,
    identity_id UUID NOT NULL,
    version BIGINT NOT NULL DEFAULT 0,
    tabs JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (sandbox_id, identity_id)
);

-- The Orchestrator writes working directories onto every layout of one sandbox
-- immediately before stopping it, and reads none of them by identity.
CREATE INDEX idx_sandbox_layouts_sandbox ON sandbox_layouts (sandbox_id);
