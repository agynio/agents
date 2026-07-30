-- An agent carried its own image and compute resources inline, duplicating what
-- an environment already is: one runtime definition an agent and a sandbox can
-- both point at. The reference is recorded here so an agent can name one.
--
-- Nullable, and null on every agent written before environments existed. Those
-- agents keep the inline image and resources they still carry, which remain in
-- place and are still read.
ALTER TABLE agents
    ADD COLUMN environment_id UUID;

-- Composite, the way sandboxes reference environments, so an agent cannot
-- reference an environment belonging to another organization. A null
-- environment_id satisfies it, which is what leaves the column optional.
ALTER TABLE agents
    ADD CONSTRAINT agents_environment_fkey FOREIGN KEY (organization_id, environment_id) REFERENCES environments (organization_id, id);

CREATE INDEX agents_environment_id_idx ON agents (environment_id);
