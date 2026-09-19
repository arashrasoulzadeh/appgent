# Local Dev Setup

## Prerequisites

- Go 1.22+
- Node.js 20+ / pnpm
- Docker + Docker Compose

## Bring up infra

```bash
cp .env.example .env
docker compose up -d postgres temporal minio
```

This starts:
- **postgres** (5432) — primary DB, also hosts `pgvector`
- **temporal** — Temporal dev server (includes its own SQLite/Postgres state
  and the Temporal Web UI on `:8233`)
- **minio** (9000/9001) — S3-compatible object storage for generated bundles

Run migrations (`internal/db/migrations`) against `POSTGRES_DSN` using
whichever migration tool is chosen (`golang-migrate` recommended):

```bash
migrate -database "$POSTGRES_DSN" -path internal/db/migrations up
```

Seed the MVP admin user (`ADMIN_SEED_EMAIL` / `ADMIN_SEED_PASSWORD` from
`.env`, bcrypt-hashed at seed time):

```bash
go run ./cmd/seed
```

## Run the services

```bash
# terminal 1
go run ./services/api

# terminal 2
go run ./services/worker

# terminal 3
cd apps/web && pnpm install && pnpm dev
```

- Next.js: http://localhost:3000
- API: http://localhost:8080
- Temporal Web UI: http://localhost:8233
- MinIO console: http://localhost:9001

## Sanity check

1. Log in at `localhost:3000` with the seeded admin credentials.
2. Create an app with a short prompt.
3. Watch `agent_steps` progress either in the dashboard (SSE) or directly in
   the Temporal Web UI under the `appgent-generation` task queue.
4. Once `status = ready`, the run's preview endpoint should resolve — see
   [open-questions.md](open-questions.md) for the current sandbox stub
   behavior if the runtime decision isn't finalized yet (early build steps
   can stub `Provisioner` to just serve the static bundle from MinIO
   directly, no real sandbox, to unblock frontend work).

## Testing

- Go: standard `go test ./...`; Temporal workflow logic should use
  `go.temporal.io/sdk/testsuite` for workflow/activity unit tests (mock
  OpenRouter calls at the `internal/openrouter` client interface).
- Next.js: component tests with your preferred RTL setup; no E2E burning
  real OpenRouter tokens in CI — mock the API layer for frontend tests.
