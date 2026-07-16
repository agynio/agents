CREATE TABLE agent_instances (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    organization_id UUID NOT NULL,
    label TEXT,
    suffix TEXT NOT NULL,
    nickname TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'paused', 'terminated')),
    pause_reason TEXT,
    last_activity_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (label IS NULL OR label = suffix),
    CHECK ((state = 'paused') OR pause_reason IS NULL)
);

CREATE UNIQUE INDEX agent_instances_unique_active_label
    ON agent_instances (agent_id, label)
    WHERE label IS NOT NULL AND state <> 'terminated';

CREATE UNIQUE INDEX agent_instances_unique_active_suffix
    ON agent_instances (agent_id, suffix)
    WHERE state <> 'terminated';

CREATE INDEX idx_agent_instances_organization ON agent_instances (organization_id, id);
CREATE INDEX idx_agent_instances_agent ON agent_instances (agent_id, id);
CREATE INDEX idx_agent_instances_state ON agent_instances (state, id);

CREATE TABLE inbox_items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_instance_id UUID NOT NULL REFERENCES agent_instances(id) ON DELETE CASCADE,
    source_kind TEXT NOT NULL CHECK (source_kind IN ('thread', 'direct')),
    thread_id UUID,
    message_id UUID,
    sender_id UUID NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    file_ids TEXT[] NOT NULL DEFAULT '{}',
    accepted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    acked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (
        (source_kind = 'thread' AND thread_id IS NOT NULL AND message_id IS NOT NULL)
        OR (source_kind = 'direct' AND thread_id IS NULL AND message_id IS NULL)
    )
);

CREATE UNIQUE INDEX inbox_items_unique_thread_delivery
    ON inbox_items (agent_instance_id, thread_id, message_id)
    WHERE source_kind = 'thread';

CREATE INDEX idx_inbox_items_unacked_fifo
    ON inbox_items (agent_instance_id, accepted_at, id)
    WHERE acked_at IS NULL;

CREATE INDEX idx_inbox_items_unacked_thread
    ON inbox_items (agent_instance_id, thread_id)
    WHERE acked_at IS NULL AND thread_id IS NOT NULL;
