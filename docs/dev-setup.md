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

This starts postgres/temporal/minio with **no host port bindings** — they're
internal-only, see [docs/environment.md](environment.md). For local dev
against them directly (e.g. `psql`, `go run ./services/api`), either add a
temporary port mapping yourself or use `docker compose exec`. For the
Temporal Web UI, bring up the authenticated proxy too:

```bash
docker compose --profile tools run --rm temporal-ui-htpasswd
docker compose up -d temporal-ui temporal-ui-proxy
```
Then it's at `http://localhost:28233`, prompting for the
`TEMPORAL_UI_USER`/`TEMPORAL_UI_PASSWORD` from `.env`.

Run migrations via the `migrate` tool service (works against the in-network
`postgres`, no host port needed):

```bash
docker compose --profile tools run --rm migrate
```

Seed the MVP admin user (`ADMIN_SEED_EMAIL` / `ADMIN_SEED_PASSWORD` from
`.env`, bcrypt-hashed at seed time):

```bash
docker compose --profile tools run --rm seed
```

## Run the services

Since postgres/temporal/minio have no host ports (see above), running
`api`/`worker` natively via `go run` won't resolve their Docker-network
hostnames. Either run them in Compose too (`docker compose up -d --build api
worker`), or temporarily add host port mappings under `postgres`/`temporal`/
`minio` in `docker-compose.yml` for local iteration and point `.env` at
`localhost:<port>` instead. `web` still runs natively fine, since it only
talks to `api` over HTTP:

```bash
# via Compose (recommended)
docker compose up -d --build api worker

# terminal 1 — web only needs api reachable at NEXT_PUBLIC_API_BASE_URL
cd apps/web && pnpm install && pnpm dev
```

- Next.js: http://localhost:3000 (running natively via `pnpm dev`, its own default port)
- API: http://localhost:28080 (running in Compose)
- Temporal Web UI: http://localhost:28233 (via the authenticated proxy)

Point `NEXT_PUBLIC_API_BASE_URL` in `.env` at `http://localhost:28080/api/v1`
to match.

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
