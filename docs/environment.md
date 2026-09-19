# Environment Variables

All services read config from env vars, loaded from `.env` locally via
Docker Compose (`env_file: .env`) and from real env vars in deployment — no
secrets manager for v1 (see [plan.md](../plan.md) §7). `.env.example` at the
repo root lists every key below with placeholder/dummy values, committed to
git; the real `.env` is gitignored.

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
| `CORS_ALLOWED_ORIGIN` | `http://localhost:3000` | Next.js dev origin |
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
| `NEXT_PUBLIC_API_BASE_URL` | `http://localhost:8080/api/v1` | |

## Deferred (sandbox runtime dependent)

Once the sandbox runtime decision ([open-questions.md](open-questions.md)) is
made, add its credentials here (e.g. `FREESTYLE_API_KEY` if Freestyle VMs are
chosen). Not yet included since the choice isn't final.
