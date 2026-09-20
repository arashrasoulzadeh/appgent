-- 0003_generation_runs.sql
CREATE TYPE run_status AS ENUM ('queued', 'running', 'succeeded', 'needs_review', 'failed');

CREATE TABLE generation_runs (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id                uuid NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    version               int NOT NULL,
    user_prompt           text NOT NULL,
    status                run_status NOT NULL DEFAULT 'queued',
    temporal_workflow_id  text NOT NULL,
    bundle_path           text,
    preview_url           text,
    error                 text,
    started_at            timestamptz,
    finished_at           timestamptz,
    created_at            timestamptz NOT NULL DEFAULT now(),
    UNIQUE (app_id, version)
);
CREATE INDEX idx_generation_runs_app_id ON generation_runs(app_id);