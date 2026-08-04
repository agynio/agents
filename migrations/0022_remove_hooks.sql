-- Hooks are removed rather than migrated: nothing is in use and no replacement
-- is offered. They were also the last resource carrying a free-form image
-- reference, which is what makes "no resource outside the catalog holds a
-- registry URL" literally true.
--
-- ENV, InitScript and VolumeAttachment now accept only agent and MCP targets.
-- Rows targeting a hook go with the hooks themselves; they addressed a
-- container that no longer exists.
DELETE FROM envs WHERE hook_id IS NOT NULL;
DELETE FROM init_scripts WHERE hook_id IS NOT NULL;
DELETE FROM volume_attachments WHERE hook_id IS NOT NULL;
DELETE FROM image_pull_secret_attachments WHERE hook_id IS NOT NULL;

ALTER TABLE envs DROP COLUMN IF EXISTS hook_id;
ALTER TABLE init_scripts DROP COLUMN IF EXISTS hook_id;
ALTER TABLE volume_attachments DROP COLUMN IF EXISTS hook_id;
ALTER TABLE image_pull_secret_attachments DROP COLUMN IF EXISTS hook_id;

DROP TABLE IF EXISTS hooks;
