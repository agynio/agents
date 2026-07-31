-- An instance had no record of the thread it was created to serve, so the text
-- an agent CLI produces at the end of a turn had nowhere to go.
--
-- Nullable: an instance whose class asked for no inference, and that nobody has
-- named a thread for since, legitimately has none. Its untargeted sends are
-- rejected rather than guessed at.
--
-- No foreign key. Threads are another service's data; this records a reference
-- across that boundary the way thread participants already do.
ALTER TABLE agent_instances
    ADD COLUMN default_thread_id UUID;

-- Where an instance's default thread comes from when the platform creates it,
-- and what becomes of a turn's final text. Both describe how an agent is
-- written rather than any one instance, so they sit on the class.
--
-- The defaults preserve today's behaviour: origin is the only inference that
-- composes for delegation, and discard keeps agents that already send
-- explicitly from posting everything twice.
ALTER TABLE agents
    ADD COLUMN default_thread TEXT NOT NULL DEFAULT 'origin',
    ADD COLUMN final_message TEXT NOT NULL DEFAULT 'discard';

ALTER TABLE agents
    ADD CONSTRAINT agents_default_thread_check CHECK (default_thread IN ('origin', 'none')),
    ADD CONSTRAINT agents_final_message_check CHECK (final_message IN ('discard', 'default_thread'));
