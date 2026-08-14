-- Whether shells in an environment's workloads outlive the connections that
-- reach them.
--
-- An environment setting rather than a per-session choice: it decides what
-- stopping a workload destroys, and everyone working in one environment should
-- get the same answer whichever client they arrived through.
--
-- Defaults true, which is both the behavior this introduces as standard and the
-- right answer for environments that predate the column.
ALTER TABLE environments
  ADD COLUMN persistent_shells BOOLEAN NOT NULL DEFAULT TRUE;
