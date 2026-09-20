-- 0006_design_patterns.sql (RAG library, pgvector)
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE design_patterns (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind       text NOT NULL,
    title      text NOT NULL,
    content    text NOT NULL,
    embedding  vector(1536) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_design_patterns_embedding ON design_patterns
    USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);