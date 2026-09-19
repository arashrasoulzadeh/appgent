# Appgent

SaaS where users sign in, describe an app, and a parallel multi-agent
pipeline (Plan / Design / Code / QA) generates a website or PWA using
OpenRouter, orchestrated by Temporal, with live preview and deploy.

## Docs

- [plan.md](plan.md) — architecture overview, decisions, build order
- [docs/api.md](docs/api.md) — REST API contract (Next.js ↔ Go)
- [docs/data-model.md](docs/data-model.md) — Postgres schema (DDL)
- [docs/temporal-workflow.md](docs/temporal-workflow.md) — workflow/activity contracts
- [docs/agents.md](docs/agents.md) — per-agent prompt specs
- [docs/environment.md](docs/environment.md) — env var reference
- [docs/dev-setup.md](docs/dev-setup.md) — local dev setup
- [docs/open-questions.md](docs/open-questions.md) — remaining unresolved decision (sandbox runtime)

## Repo layout

```
appgent/
├── apps/web/          # Next.js frontend
├── services/api/      # Go API server
├── services/worker/   # Go Temporal worker
├── internal/          # shared Go packages (agents, db, openrouter, temporal, storage, sandbox)
├── infra/             # local dev infra config
├── docs/              # design docs (see above)
├── docker-compose.yml
└── .env.example
```

## Quickstart

See [docs/dev-setup.md](docs/dev-setup.md).

## Status

Planning complete. Implementation not yet started — begin at plan.md §9
step 1.
