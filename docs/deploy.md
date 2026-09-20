# Deploying to a Server (single VPS, Docker Compose)

Everything — infra (Postgres/Temporal/MinIO) and app services (api/worker/web)
— runs via one `docker-compose.yml`. `api`, `worker`, and `web` now have
Dockerfiles (`services/api/Dockerfile`, `services/worker/Dockerfile`,
`apps/web/Dockerfile`).

## 1. Provision the server

Any VPS with Docker + Docker Compose plugin installed (Ubuntu 22.04+
recommended). Open ports `80`/`443` (reverse proxy) — do not expose the
infra/app host ports (`25432`, `27233`, `28233`, `28080`, `29000`/`29001`,
`23000`) publicly; they're for internal/compose-network use and admin access
only (tunnel or firewall-restrict them). These are deliberately non-standard
port numbers to avoid colliding with anything else already running on a
shared host — see [docs/environment.md](environment.md) for the full table.

## 2. Get the code onto the server

```bash
git clone <your-repo-url> appgent
cd appgent
```

## 3. Configure `.env`

```bash
cp .env.example .env
```

Edit `.env` and set **real production values**, at minimum:
- `JWT_SIGNING_SECRET` — long random value, never the placeholder
- `ADMIN_SEED_PASSWORD` — change from `admin`
- `AI_API_KEY` / `OPENROUTER_API_KEY` — your real OpenRouter key
- `CORS_ALLOWED_ORIGIN` — your real domain, e.g. `https://appgent.example.com`
- `NEXT_PUBLIC_API_BASE_URL` — your real public API URL, e.g. `https://appgent.example.com/api/v1`

`POSTGRES_DSN` set inside `docker-compose.yml` for the `api`/`worker`
services already points at the in-network `postgres` host — you don't need
to edit that one for the compose deployment; it's only relevant if you run
`api`/`worker` outside Docker.

## 4. Bring up infra, then migrate + seed

```bash
docker compose up -d postgres temporal temporal-ui minio
docker compose --profile tools run --rm migrate
docker compose --profile tools run --rm seed
```

## 5. Build and start the app services

```bash
docker compose up -d --build api worker web
```

Check logs:
```bash
docker compose logs -f api worker web
```

## 6. Put a reverse proxy in front

Use nginx or Caddy on the host (or a small proxy container) to terminate TLS
and route:
- `/` and other frontend paths → `web:3000`
- `/api/*` → `api:8080` (or keep `NEXT_PUBLIC_API_BASE_URL` pointed at a
  separate `api.` subdomain and skip path-based routing — either works)

Example Caddy config (`Caddyfile`, run as its own container or on the host):
```
appgent.example.com {
    reverse_proxy /api/* api:8080
    reverse_proxy web:3000
}
```
Caddy handles Let's Encrypt TLS automatically when given a real domain.

## 7. Verify

```bash
curl https://appgent.example.com/api/v1/healthz
```
Then log in at `https://appgent.example.com` with the seeded admin
credentials and create a test app.

## Updating a deployment

```bash
git pull
docker compose --profile tools run --rm migrate   # if new migrations exist
docker compose up -d --build api worker web
```

## Notes

- The `migrate` and `seed` services use Compose `profiles: ["tools"]` so
  `docker compose up -d` never runs them automatically — invoke them
  explicitly as one-off commands.
- Sandbox/preview provisioning (§ [open-questions.md](open-questions.md))
  isn't part of this compose file yet — resolving that decision may add
  another service or external dependency here.
- Back up the `postgres_data` volume regularly; it holds all user/app/run
  state. `minio_data` holds generated bundles.
