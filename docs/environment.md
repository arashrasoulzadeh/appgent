# Environment Variables

All services read config from env vars, loaded from `.env` locally via
Docker Compose (`env_file: .env`) and from real env vars in deployment — no
secrets manager for v1 (see [plan.md](../plan.md) §7). `.env.example` at the
repo root lists every key below with placeholder/dummy values, committed to
git; the real `.env` is gitignored.

## Docker Compose host ports

`docker-compose.yml` intentionally uses non-standard host ports (right side
of `host:container` below is the container's own default port, unchanged) to
avoid colliding with other services already running on a shared box:

| Service | Host port | Container port | Purpose |
|---|---|---|---|
| postgres | `15432` | `5432` | direct DB access (admin/debugging only) |
| temporal | `17233` | `7233` | Temporal gRPC frontend |
| temporal-ui | `18233` | `8080` | Temporal Web UI |
| minio | `19000` | `9000` | S3 API |
| minio | `19001` | `9001` | MinIO console |
| api | `18080` | `8080` | REST API (put behind reverse proxy for public access) |
| web | `13000` | `3000` | Next.js (put behind reverse proxy for public access) |

Container-to-container env vars (`POSTGRES_DSN`, `TEMPORAL_HOST_PORT`,
`OBJECT_STORAGE_ENDPOINT` below) always use the **container** port and the
Compose service name as host — the host-port table above only matters for
things reached from outside Docker's network (your browser, `psql` from the
host, a reverse proxy).

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
| `CORS_ALLOWED_ORIGIN` | `http://localhost:13000` | Next.js origin (or your public domain in prod) |
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
| `NEXT_PUBLIC_API_BASE_URL` | `http://localhost:18080/api/v1` | or your public API domain in prod |

## Deferred (sandbox runtime dependent)

Once the sandbox runtime decision ([open-questions.md](open-questions.md)) is
made, add its credentials here (e.g. `FREESTYLE_API_KEY` if Freestyle VMs are
chosen). Not yet included since the choice isn't final.
