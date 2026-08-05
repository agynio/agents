-- Environments and MCPs name catalog images instead of holding a registry
-- address. Added alongside the free-form `image` columns rather than replacing
-- them: nothing reads the new ones yet, and migration is manual - an
-- environment shared by agents with different init images becomes several
-- environments, which is a fan-out rather than a field rewrite.
ALTER TABLE environments
    ADD COLUMN IF NOT EXISTS workspace_image_id UUID,
    ADD COLUMN IF NOT EXISTS workspace_image_tag TEXT NOT NULL DEFAULT '',
    -- Null means a workspace-only environment: usable by sandboxes, rejected
    -- by CreateAgent.
    ADD COLUMN IF NOT EXISTS agent_runtime_image_id UUID,
    ADD COLUMN IF NOT EXISTS agent_runtime_image_tag TEXT NOT NULL DEFAULT '';

ALTER TABLE mcps
    ADD COLUMN IF NOT EXISTS image_id UUID,
    ADD COLUMN IF NOT EXISTS image_tag TEXT NOT NULL DEFAULT '';

-- image stays NOT NULL with an empty default rather than becoming nullable: a
-- record written through the catalog path leaves it empty, and every reader
-- still scans it into a string.
ALTER TABLE environments ALTER COLUMN image SET DEFAULT '';
ALTER TABLE mcps ALTER COLUMN image SET DEFAULT '';

-- No foreign key on the image columns: images live in another service, and a
-- deleted image is a condition to surface rather than a write to block. An
-- environment naming one becomes unschedulable, the same treatment an
-- unresolvable flavor name gets.
