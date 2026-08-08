-- How workloads in an environment reach an LLM. The mode lives here rather than
-- on the agent because a sandbox has no agent to read it from, and because it
-- decides what the orchestrator stamps on the workload identity.
ALTER TABLE environments ADD COLUMN llm_mode TEXT NOT NULL DEFAULT 'platform';
ALTER TABLE environments ADD COLUMN llm_allowed_models TEXT[] NOT NULL DEFAULT '{}';

ALTER TABLE environments ADD CONSTRAINT environments_llm_mode_check
    CHECK (llm_mode IN ('platform', 'native'));

-- The vendor's own model name, for native mode. Opaque to the platform: the
-- namespace is the vendor's, so a wrong value fails at the vendor rather than
-- here. Mutually exclusive with model, which the environment's mode decides.
ALTER TABLE agents ADD COLUMN model_name TEXT NOT NULL DEFAULT '';
