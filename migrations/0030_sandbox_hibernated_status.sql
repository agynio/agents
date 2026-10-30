-- A sandbox asleep on a runner that can hibernate: its workload frozen with its
-- shells, processes and scratch space, and woken in place by the next connect.
-- Its record keeps the workload it will wake, so it is not stopped.
--
-- Replaces the unnamed status check from 0014, which PostgreSQL named
-- sandboxes_status_check.
ALTER TABLE sandboxes
    DROP CONSTRAINT sandboxes_status_check;

ALTER TABLE sandboxes
    ADD CONSTRAINT sandboxes_status_check
        CHECK (status IN ('starting', 'running', 'hibernated', 'stopped', 'failed', 'terminated'));
