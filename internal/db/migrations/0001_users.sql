-- 0001_users.sql
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text UNIQUE NOT NULL,
    password_hash text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- MVP seed: INSERT INTO users (email, password_hash) VALUES ('admin', '<bcrypt of "admin">');
-- Run via seed binary instead