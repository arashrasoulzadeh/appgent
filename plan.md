# Appgent — Project Plan

SaaS where users sign in, describe an app, and a parallel multi-agent pipeline
(Plan / Code / QA / Design) generates a website or PWA using OpenRouter LLMs,
orchestrated by Temporal, with live preview and deploy to our own hosting.

## 1. Architecture Overview

```
┌─────────────┐      REST/gRPC       ┌──────────────────────┐
│   Next.js    │ ───────────────────▶ │   Go API Server       │
│  (frontend)  │ ◀─────────────────── │  (auth, apps, runs)   │
└─────────────┘      JSON/SSE         └──────────┬────────────┘
                                                   │ starts workflow
                                                   ▼
                                        ┌──────────────────────┐
                                        │  Temporal Server      │
                                        │  (workflow engine)    │
                                        └──────────┬────────────┘
                                                   │ schedules activities
                                                   ▼
                                        ┌──────────────────────┐
                                        │  Go Temporal Worker   │
                                        │  Plan / Code / QA /   │
                                        │  Design activities    │
                                        │  → OpenRouter LLM     │
                                        └──────────┬────────────┘
                                                   │ writes files
                                                   ▼
                                ┌───────────────────────────────────┐
                                │ Object Storage (generated bundles) │
                                │ Sandbox runtime (preview/deploy)   │
                                └───────────────────────────────────┘

Postgres: users, apps, generation runs, agent step logs, sessions
Vector DB: design patterns / component snippets / prior-run RAG context
```

**Split of responsibility**
- **Next.js**: pure frontend — sign-in UI, dashboard, chat/prompt UI for app creation, live preview iframe, deploy button. No business logic, no direct DB access. Calls the Go API only.
- **Go API server**: REST/gRPC API, owns Postgres, issues/validates sessions, starts Temporal workflows, streams run progress back to the frontend (SSE or WebSocket), serves generated-app metadata and deploy status.
- **Go Temporal worker**: hosts the actual workflow + activity code for the generation pipeline. Same Go module as the API server (shared types), separate deployable process(es) so it scales independently.

## 2. Temporal Workflow Design

**Workflow**: `GenerateAppWorkflow(runID, userPrompt, appID)`

Parallel-with-dependency graph (not strictly sequential):

```
                 ┌───────────────┐
                 │  Plan Agent    │  (activity: PlanActivity)
                 │ breaks prompt  │  → produces: page list, component list,
                 │ into a spec    │     data model, style direction
                 └───────┬────────┘
                          │ spec
             ┌────────────┴─────────────┐
             ▼                          ▼
   ┌──────────────────┐       ┌──────────────────┐
   │  Design Agent     │       │  Code Agent       │
   │ produces: tokens, │       │ produces: Next.js │  ← runs in parallel
   │ layout, copy tone │       │ site/PWA code      │     (Design output is
   └─────────┬─────────┘       └─────────┬─────────┘      streamed to Code
             │                            │                as it's ready,
             └─────────────┬──────────────┘                via Temporal Signal)
                            ▼
                 ┌───────────────────┐
                 │   QA Agent          │  (activity: QAActivity)
                 │ lint / build check / │
                 │ broken-link check /  │
                 │ a11y basics /        │
                 │ manifest + service   │
                 │ worker validation    │  (for kind=pwa: checks manifest.json,
                 └─────────┬─────────────┘   SW registration, offline strategy)
                            │ pass ──────────────▶ Preview (deploy to sandbox)
                            │ fail (retry loop, max N)
                            ▼
                 back to Code Agent with QA feedback
```

- Each agent activity is a Temporal Activity that calls OpenRouter with a role-specific system prompt + shared run context, writes its output to object storage + Postgres (`agent_steps` table) for auditability.
- Design and Code activities run **concurrently** as Temporal `Future`s; Code activity waits on a Signal carrying the Design tokens once ready (or times out and proceeds with defaults).
- QA failure triggers a bounded retry: QA feedback is fed back into the Code Agent (max 2–3 retries), then the workflow either succeeds or marks the run `needs_review`.
- Workflow emits progress via Temporal Queries (polled by the Go API) or Updates, which the API relays to the frontend over SSE.
- All activity code (OpenRouter calls, prompt templates) lives in a shared `internal/agents` package so activities stay thin and testable.

## 3. Data Model (Postgres)

- `users` (id, email, password_hash, created_at) — MVP: single seeded admin/admin user, real signup deferred
- `apps` (id, user_id, name, kind [website|pwa], status, created_at)
- `generation_runs` (id, app_id, status, started_at, finished_at, error)
- `agent_steps` (id, run_id, agent_type [plan|design|code|qa], input, output, model_used, tokens_used, started_at, finished_at, status)
- `deployments` (id, app_id, run_id, url, status, deployed_at)

Object storage: generated file bundles per run (`runs/{run_id}/...`), keyed so a run's output is immutable and re-runs create new versions.

Vector DB: embeddings of reusable design patterns / component snippets / past successful specs, queried by the Plan and Design agents as RAG context (not user data — a shared library that improves over time).

## 4. Preview & Deploy

- Preview and deploy runtime: **not yet decided** — leaning toward Freestyle VMs (sandbox-as-a-service, matches "sandboxed container per app" requirement with least infra to build ourselves) vs. self-managed Firecracker/Docker. **Open question, see §7.**
- Preview: after QA pass, bundle is provisioned into a sandbox, given a short-lived preview URL, iframed into the Next.js dashboard.
- Deploy: on user action, promote the same bundle to persistent hosting under our domain (subdomain per app, e.g. `{app-slug}.appgent.app`), custom domain support later.

## 5. Auth (MVP)

- No real signup flow yet. Single seeded user: `admin` / `admin`, stored as a bcrypt hash in `users`, session via signed JWT cookie issued by the Go API.
- Structured so real auth (NextAuth.js / Auth.js with OAuth) can replace this later without changing the API contract (`POST /auth/login` stays the same shape).

## 6. Repo Layout

```
appgent/
├── apps/
│   └── web/                 # Next.js frontend
├── services/
│   ├── api/                 # Go API server (cmd/api)
│   └── worker/               # Go Temporal worker (cmd/worker)
├── internal/                # shared Go packages
│   ├── agents/               # plan/code/qa/design activity logic + prompts
│   ├── db/                   # Postgres models/queries
│   ├── openrouter/           # OpenRouter client
│   ├── temporal/             # workflow + activity definitions
│   └── storage/              # object storage client
├── infra/                    # docker-compose for local dev (Postgres, Temporal, MinIO)
├── plan.md
└── README.md
```

## 7. Decisions

- **Model strategy**: all four agents (Plan/Design/Code/QA) use **Nemotron 3 Ultra (free tier)** via OpenRouter to start. Cheap enough that per-agent tiering isn't needed yet; swap individual agents to a stronger paid model later if quality on Code/Design falls short — the `internal/openrouter` client takes model as a per-call param so this is a config change, not a refactor.
- **Vector DB**: `pgvector` extension in the same Postgres instance. No new service.
- **QA failure UX**: after max retries, the run is marked `needs_review`, the partial generated app is still shown in preview, and the QA agent's failure list is surfaced in the run timeline with a "regenerate" action.
- **Billing/usage limits**: out of scope for MVP (single seeded admin user). Add per-user spend caps (tracked via `agent_steps.tokens_used`, already in the schema) before opening real signups.
- **API protocol**: REST + JSON between Next.js and the Go API; run progress streamed to the dashboard via Server-Sent Events (no gRPC/codegen toolchain for v1).
- **Post-generation editing**: regenerate-only in v1 — no in-browser code editor. Users iterate by re-prompting; QA-failure fixes go through the same agent loop, not manual edits.
- **Versioning**: full run history is kept. Every regenerate creates a new `generation_runs` row + new bundle version; users can view, preview, or restore any past version (already fits the schema in §3, no extra design needed).
- **Infra target for this project**: Docker Compose locally (Postgres, Temporal, MinIO — already in §6), deployed as containers on a single host/small cluster for v1. No Kubernetes until scale requires it.
- **PWA depth**: full offline-capable PWA — service worker with a real cache-first/network-first strategy per route and background sync where applicable. This is a real capability the Plan/Design/Code agents must reason about explicitly (which routes/data are cacheable, what "offline" means for the app being generated), and QA must check the manifest + service worker registration, not just build/lint. Bumps prompt and QA complexity vs. a cosmetic PWA flag — worth calling out as pipeline scope, not just a checkbox.
- **Concurrency/queueing**: rely on Temporal's built-in task-queue concurrency limits and per-user workflow rate limiting; no separate queue/credit system or UI for v1.
- **Secrets management**: env vars / `.env` + Docker Compose secrets for the OpenRouter key and other credentials. No external secrets manager for v1.
- **Observability**: structured JSON logs from the Go services + Temporal's own Web UI for workflow/activity visibility. No separate metrics/tracing stack at launch.

## 8. Open Question (still unresolved)

1. **Sandbox runtime for preview/deploy** — Freestyle VMs vs. self-managed containers (Firecracker/Docker) vs. static-export-only. Left open; the build order (§9) stubs preview first and resolves this at step 8, once the generation pipeline itself works end-to-end.

## 9. Build Order (proposed)

1. Repo scaffolding: Go modules, Next.js app, docker-compose (Postgres, Temporal dev server, MinIO for object storage)
2. Go API server: seeded admin auth, `apps` CRUD, health checks
3. Next.js: login page, dashboard shell, "new app" prompt form
4. Temporal worker skeleton: `GenerateAppWorkflow` with stub activities (no LLM yet) wired end-to-end, progress visible in UI
5. OpenRouter client + real Plan activity, then Code activity (sequential first, prove the loop)
6. Add Design activity running in parallel with Code (Signal-based handoff)
7. Add QA activity + retry loop
8. Preview: pick sandbox runtime (resolve open question in §8), wire bundle → preview URL
9. Deploy: promote preview to persistent hosting
10. Vector DB + RAG context for Plan/Design agents
11. Polish: run history UI, agent step logs/timeline view, error states

## 10. Supporting Documents

Detailed specs an implementing agent needs, split out of this file:

- [docs/api.md](docs/api.md) — full REST API contract (endpoints, request/response shapes)
- [docs/data-model.md](docs/data-model.md) — Postgres schema as runnable DDL
- [docs/temporal-workflow.md](docs/temporal-workflow.md) — workflow/activity Go contracts, retry policy, queries
- [docs/agents.md](docs/agents.md) — prompt specs for Plan/Design/Code/QA agents
- [docs/environment.md](docs/environment.md) — every env var, per service
- [docs/dev-setup.md](docs/dev-setup.md) — local dev bring-up and sanity check
- [docs/open-questions.md](docs/open-questions.md) — sandbox runtime decision + the `Provisioner` interface that keeps it swappable
- [docker-compose.yml](docker-compose.yml) / [.env.example](.env.example) — local infra scaffolding

---
*Generated as a planning doc — implementation has not started. Only the sandbox runtime (§8) remains open; begin at §9 step 1.*
