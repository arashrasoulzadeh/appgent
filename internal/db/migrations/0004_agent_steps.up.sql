-- 0004_agent_steps.sql
CREATE TYPE agent_type AS ENUM ('plan', 'design', 'code', 'qa');
CREATE TYPE agent_step_status AS ENUM ('pending', 'running', 'succeeded', 'failed');

CREATE TABLE agent_steps (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id       uuid NOT NULL REFERENCES generation_runs(id) ON DELETE CASCADE,
    agent_type   agent_type NOT NULL,
    attempt      int NOT NULL DEFAULT 1,
    input        jsonb NOT NULL,
    output       jsonb,
    model_used   text NOT NULL,
    tokens_used  int,
    status       agent_step_status NOT NULL DEFAULT 'pending',
    error        text,
    started_at   timestamptz,
    finished_at  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_agent_steps_run_id ON agent_steps(run_id);