-- 0005_deployments.sql
CREATE TYPE deployment_status AS ENUM ('deploying', 'live', 'failed', 'retired');

CREATE TABLE deployments (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id       uuid NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    run_id       uuid NOT NULL REFERENCES generation_runs(id) ON DELETE CASCADE,
    url          text,
    status       deployment_status NOT NULL DEFAULT 'deploying',
    deployed_at  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_deployments_app_id ON deployments(app_id);