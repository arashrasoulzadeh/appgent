# Environment Variables

All services read config from env vars, loaded from `.env` locally via
Docker Compose (`env_file: .env`) and from real env vars in deployment — no
secrets manager for v1 (see [plan.md](../plan.md) §7). `.env.example` at the
repo root lists every key below with placeholder/dummy values, committed to
git; the real `.env` is gitignored.

## Docker Compose host ports

Only three services are reachable from outside Docker's network — everything
else (postgres, temporal's gRPC frontend, minio) is internal-only, with no
host port binding at all. This is deliberate: those services have no auth of
their own worth exposing publicly, so they're only reachable from other
containers on the compose network (or via `docker compose exec`/`run` for
admin access).

| Service | Host port | Container port | Purpose | Auth |
|---|---|---|---|---|
| web | `23000` | `3000` | Next.js frontend | app login |
| api | `28080` | `8080` | REST API | JWT session cookie |
| temporal-ui-proxy | `28233` | `8080` (via nginx) | Temporal Web UI | HTTP Basic Auth (`TEMPORAL_UI_USER`/`_PASSWORD`) |

`postgres`, `temporal`, and `minio` have **no** `ports:` mapping — reach them
only via `docker compose exec postgres psql ...`, `docker compose exec minio
...`, etc., or by SSH-tunneling to the host and using `docker compose exec`
there. `temporal-ui` itself also has no host port; `temporal-ui-proxy`
(nginx, see [infra/temporal-ui-proxy](../infra/temporal-ui-proxy)) is the
only way to reach it from a browser, and it requires HTTP Basic Auth.

Container-to-container env vars (`POSTGRES_DSN`, `TEMPORAL_HOST_PORT`,
`OBJECT_STORAGE_ENDPOINT` below) always use the **container** port and the
Compose service name as host, unaffected by any of this.

## Shared

| Var | Example | Used by |
|---|---|---|
| `POSTGRES_DSN` | `postgres://appgent:appgent@postgres:5432/appgent?sslmode=disable` | api, worker |
| `TEMPORAL_HOST_PORT` | `temporal:7233` | api, worker |
| `TEMPORAL_NAMESPACE` | `default` | api, worker |
| `OBJECT_STORAGE_ENDPOINT` | `http://minio:9000` | api, worker |
| `OBJECT_STORAGE_BUCKET` | `appgent-runs` | api, worker |
| `OBJECT_STORAGE_ACCESS_KEY` | `minioadmin` | api, worker |
| `OBJECT_STORAGE_SECRET_KEY` | `minioadmin` | api, worker |
| `LOG_LEVEL` | `info` | api, worker |

## API server (`services/api`)

| Var | Example | Notes |
|---|---|---|
| `API_PORT` | `8080` | |
| `JWT_SIGNING_SECRET` | `change-me-in-prod` | signs the session cookie |
| `SESSION_COOKIE_NAME` | `session` | |
| `SESSION_TTL_HOURS` | `168` | |
| `CORS_ALLOWED_ORIGIN` | `http://localhost:23000` | Next.js origin (or your public domain in prod) |
| `ADMIN_SEED_EMAIL` | `admin` | used by the seed migration/script |
| `ADMIN_SEED_PASSWORD` | `admin` | plaintext only in local `.env`; hashed at seed time, never stored plain |

## Worker (`services/worker`)

| Var | Example | Notes |
|---|---|---|
| `OPENROUTER_API_KEY` | `sk-or-...` | required |
| `OPENROUTER_BASE_URL` | `https://openrouter.ai/api/v1` | |
| `OPENROUTER_MODEL_PLAN` | `nvidia/nemotron-3-ultra:free` | |
| `OPENROUTER_MODEL_DESIGN` | `nvidia/nemotron-3-ultra:free` | |
| `OPENROUTER_MODEL_CODE` | `nvidia/nemotron-3-ultra:free` | |
| `OPENROUTER_MODEL_QA` | `nvidia/nemotron-3-ultra:free` | |
| `MAX_QA_RETRIES` | `3` | |
| `WORKER_MAX_CONCURRENT_ACTIVITIES` | `10` | |

## Next.js (`apps/web`)

| Var | Example | Notes |
|---|---|---|
| `NEXT_PUBLIC_API_BASE_URL` | `http://localhost:28080/api/v1` | or your public API domain in prod — see note below |

`NEXT_PUBLIC_*` vars are inlined into the JS bundle at `next build` time, not
read at container runtime. `docker-compose.yml` passes this one through as a
Docker build arg (`build.args`) as well as a runtime env var, so changing it
in `.env` requires rebuilding the `web` image (`docker compose up -d --build
web`), not just restarting the container — a plain restart keeps whatever
value was baked in at the last build.

## Temporal UI auth

| Var | Example | Notes |
|---|---|---|
| `TEMPORAL_UI_USER` | `admin` | Basic Auth username for `temporal-ui-proxy` |
| `TEMPORAL_UI_PASSWORD` | `change-me-in-prod` | Basic Auth password — change from the placeholder before deploying |

These are consumed by the one-off `temporal-ui-htpasswd` tool service, which
writes `/auth/.htpasswd` into a shared volume that `temporal-ui-proxy` (nginx)
reads. Run it once (and again whenever you change these values):
```bash
docker compose --profile tools run --rm temporal-ui-htpasswd
docker compose up -d temporal-ui-proxy
```

## Deferred (sandbox runtime dependent)

Once the sandbox runtime decision ([open-questions.md](open-questions.md)) is
made, add its credentials here (e.g. `FREESTYLE_API_KEY` if Freestyle VMs are
chosen). Not yet included since the choice isn't final.
