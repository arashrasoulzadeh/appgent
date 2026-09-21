# Data Model

Postgres is the single relational store (also backs `pgvector` for RAG — see
§ vector tables below). All tables use `uuid` primary keys (`gen_random_uuid()`,
requires `pgcrypto`) and `timestamptz` for all timestamps.

Run these as sequential migrations (e.g. via `golang-migrate` or `goose`) in
`services/api/migrations/` or a shared `internal/db/migrations/` directory —
pick one and keep both API and worker pointed at it, since both read/write
these tables.

```sql
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS vector;

-- 0001_users.sql
CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text UNIQUE NOT NULL,
    password_hash text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);
-- MVP seed: INSERT INTO users (email, password_hash) VALUES ('admin', '<bcrypt of "admin">');

-- 0002_apps.sql
CREATE TYPE app_kind AS ENUM ('website', 'pwa');
CREATE TYPE app_status AS ENUM ('draft', 'generating', 'ready', 'needs_review', 'failed');

CREATE TABLE apps (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       text NOT NULL,
    slug       text UNIQUE NOT NULL,       -- used for preview/deploy subdomain
    kind       app_kind NOT NULL,
    status     app_status NOT NULL DEFAULT 'draft',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_apps_user_id ON apps(user_id);

-- 0003_generation_runs.sql
CREATE TYPE run_status AS ENUM ('queued', 'running', 'succeeded', 'needs_review', 'failed');

CREATE TABLE generation_runs (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id        uuid NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    version       int NOT NULL,             -- 1, 2, 3... per app, assigned on insert
    user_prompt   text NOT NULL,
    status        run_status NOT NULL DEFAULT 'queued',
    temporal_workflow_id text NOT NULL,      -- for querying/signalling the workflow
    bundle_path   text,                      -- object storage key once code exists, e.g. "runs/{run_id}/"
    error         text,
    started_at    timestamptz,
    finished_at   timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (app_id, version)
);
CREATE INDEX idx_generation_runs_app_id ON generation_runs(app_id);

-- 0004_agent_steps.sql
CREATE TYPE agent_type AS ENUM ('plan', 'design', 'code', 'qa');
CREATE TYPE agent_step_status AS ENUM ('pending', 'running', 'succeeded', 'failed');

CREATE TABLE agent_steps (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id       uuid NOT NULL REFERENCES generation_runs(id) ON DELETE CASCADE,
    agent_type   agent_type NOT NULL,
    attempt      int NOT NULL DEFAULT 1,     -- QA retry loop increments this per agent_type
    input        jsonb NOT NULL,             -- prompt context sent to the model
    output       jsonb,                      -- structured agent output (spec/tokens/files/qa report)
    model_used   text NOT NULL,              -- OpenRouter model id, e.g. "nvidia/nemotron-3-ultra-550b-a55b:free"
    tokens_used  int,
    status       agent_step_status NOT NULL DEFAULT 'pending',
    error        text,
    started_at   timestamptz,
    finished_at  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_agent_steps_run_id ON agent_steps(run_id);

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

-- 0006_design_patterns.sql  (RAG library, pgvector)
CREATE TABLE design_patterns (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind       text NOT NULL,          -- "component" | "layout" | "copy_style" | "spec_example"
    title      text NOT NULL,
    content    text NOT NULL,          -- the snippet/pattern itself, fed back into prompts verbatim
    embedding  vector(1536) NOT NULL,  -- dimension depends on chosen embedding model, adjust to match
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_design_patterns_embedding ON design_patterns
    USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
```

## Notes for the implementing agent

- `generation_runs.version` is assigned by the API layer inside a transaction
  (`SELECT max(version)+1 ... FOR UPDATE` on the app row, or a Postgres
  sequence per app) — never let the client supply it.
- `agent_steps.attempt` lets the QA retry loop (see
  [temporal-workflow.md](temporal-workflow.md)) store every Code-agent retry
  without overwriting prior attempts, so the run timeline UI can show the
  full back-and-forth.
- `apps.slug` is the preview/deploy subdomain (`{slug}.appgent.app`) —
  generate from `name` + random suffix, validate DNS-safe characters, enforce
  uniqueness at insert time.
- `design_patterns` starts empty; seeding it is a later backlog item, not
  part of MVP build order step 10. The Plan/Design agents should treat an
  empty result set as "no RAG context" and proceed with base prompts only.
