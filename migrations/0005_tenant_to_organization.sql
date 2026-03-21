ALTER TABLE agents
    RENAME COLUMN tenant_id TO organization_id;
ALTER INDEX agents_tenant_id_idx RENAME TO agents_organization_id_idx;

ALTER TABLE volumes
    RENAME COLUMN tenant_id TO organization_id;
ALTER INDEX volumes_tenant_id_idx RENAME TO volumes_organization_id_idx;

DROP INDEX mcps_tenant_agent_idx;
ALTER TABLE mcps DROP COLUMN tenant_id;
CREATE INDEX mcps_agent_id_idx ON mcps (agent_id);

DROP INDEX skills_tenant_agent_idx;
ALTER TABLE skills DROP COLUMN tenant_id;
CREATE INDEX skills_agent_id_idx ON skills (agent_id);

DROP INDEX hooks_tenant_agent_idx;
ALTER TABLE hooks DROP COLUMN tenant_id;
CREATE INDEX hooks_agent_id_idx ON hooks (agent_id);

DROP INDEX volume_attachments_tenant_id_idx;
ALTER TABLE volume_attachments DROP COLUMN tenant_id;

DROP INDEX envs_tenant_id_idx;
ALTER TABLE envs DROP COLUMN tenant_id;
CREATE INDEX envs_agent_id_idx ON envs (agent_id);
CREATE INDEX envs_mcp_id_idx ON envs (mcp_id);
CREATE INDEX envs_hook_id_idx ON envs (hook_id);

DROP INDEX init_scripts_tenant_id_idx;
ALTER TABLE init_scripts DROP COLUMN tenant_id;
CREATE INDEX init_scripts_agent_id_idx ON init_scripts (agent_id);
CREATE INDEX init_scripts_mcp_id_idx ON init_scripts (mcp_id);
CREATE INDEX init_scripts_hook_id_idx ON init_scripts (hook_id);
