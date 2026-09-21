-- 0007_agent_steps_publish.sql
-- Publishing a run's generated files (build + upload to object storage)
-- is now tracked as its own agent_steps row, same as plan/design/code/qa,
-- so the frontend's existing run-detail timeline shows build progress and
-- output instead of the step being invisible.
ALTER TYPE agent_type ADD VALUE 'publish';
