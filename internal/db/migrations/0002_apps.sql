-- 0002_apps.sql
CREATE TYPE app_kind AS ENUM ('website', 'pwa');
CREATE TYPE app_status AS ENUM ('draft', 'generating', 'ready', 'needs_review', 'failed');

CREATE TABLE apps (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       text NOT NULL,
    slug       text UNIQUE NOT NULL,
    kind       app_kind NOT NULL,
    status     app_status NOT NULL DEFAULT 'draft',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_apps_user_id ON apps(user_id);