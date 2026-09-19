# Appgent — Implementation Phases

## Phase 0: Foundation & Scaffolding
- [ ] 0.1 Initialize Go module (`appgent/`) and shared internal packages structure
- [ ] 0.2 Initialize Next.js app in `apps/web/` with TypeScript, Tailwind, App Router
- [ ] 0.3 Create `docker-compose.yml` (Postgres+pgvector, Temporal, MinIO) — **DONE**
- [ ] 0.4 Create `.env.example` with all required vars — **DONE**
- [ ] 0.5 Set up Postgres migrations directory (`internal/db/migrations/`)
- [ ] 0.6 Write migration 0001_users.sql through 0006_design_patterns.sql
- [ ] 0.7 Create seed binary (`cmd/seed/main.go`) for MVP admin user
- [ ] 0.8 Verify infra starts: `docker compose up -d && migrate up && go run ./cmd/seed`

## Phase 1: Go API Server — Core
- [ ] 1.1 Project layout: `services/api/` with `cmd/api/main.go`
- [ ] 1.2 DB connection pool + query helpers (`internal/db/`)
- [ ] 1.3 JWT auth: signup (stub), login, logout, me endpoints
- [ ] 1.4 `POST /auth/login` — issues HttpOnly session cookie
- [ ] 1.5 `GET /healthz` — DB connectivity check
- [ ] 1.6 `GET /apps`, `POST /apps`, `GET /apps/{id}`, `DELETE /apps/{id}`
- [ ] 1.7 `POST /apps/{id}/regenerate` — creates new run version
- [ ] 1.8 Run listing: `GET /apps/{id}/runs`
- [ ] 1.9 Run detail: `GET /apps/{id}/runs/{run_id}` with agent steps
- [ ] 1.10 SSE endpoint: `GET /apps/{id}/runs/{run_id}/events` (poll Temporal Query or Postgres LISTEN/NOTIFY)
- [ ] 1.11 Deployments: `POST /apps/{id}/deploy`, `GET /apps/{id}/deployments`
- [ ] 1.12 Preview: `GET /apps/{id}/runs/{run_id}/preview` (stub Provisioner for now)
- [ ] 1.13 CORS, structured JSON logging, error handling middleware

## Phase 2: Next.js Frontend — Core
- [ ] 2.1 Login page (`/login`) — calls `POST /auth/login`
- [ ] 2.2 Auth context + protected routes, session persistence
- [ ] 2.3 Dashboard shell (`/dashboard`) — app list, create button
- [ ] 2.4 "New App" modal/page — name, kind (website/pwa), prompt textarea
- [ ] 2.5 App detail page — run history table, status badges
- [ ] 2.6 Run detail view — agent step timeline (Plan/Design/Code/QA), expandable logs
- [ ] 2.7 SSE connection to `/runs/{run_id}/events` — live progress updates
- [ ] 2.8 Preview iframe — loads `/preview` URL when run succeeds
- [ ] 2.9 Deploy button — calls `POST /deploy`, shows deployment status/URL
- [ ] 2.10 Regenerate action — calls `POST /regenerate` with optional new prompt
- [ ] 2.11 Responsive layout, dark mode support, loading/error states

## Phase 3: Temporal Worker — Skeleton
- [ ] 3.1 Project layout: `services/worker/` with `cmd/worker/main.go`
- [ ] 3.2 Register `GenerateAppWorkflow` with stub activities
- [ ] 3.3 Activity stubs: Plan, Design, Code, QA, PersistRunResult
- [ ] 3.4 Workflow executes activities sequentially (no parallel yet)
- [ ] 3.5 Activity options: timeouts, retry policy (transient failures)
- [ ] 3.6 Query handler: `status` for live progress
- [ ] 3.7 Worker config: `MaxConcurrentActivityExecutionSize` from env
- [ ] 3.8 End-to-end test: API starts workflow → worker runs → status updates → UI shows progress

## Phase 4: OpenRouter Client & Plan Activity
- [ ] 4.1 `internal/openrouter/client.go` — HTTP client, request/response, model param
- [ ] 4.2 Plan prompt template (`internal/agents/prompts/plan.tmpl`)
- [ ] 4.3 `PlanActivity` — calls OpenRouter, parses JSON to `PlanOutput`
- [ ] 4.4 Persist `agent_steps` row on success (input/output/model/tokens)
- [ ] 4.5 RAG context: pgvector similarity search for `DesignPattern` (empty for MVP)
- [ ] 4.6 Integration test: real OpenRouter call → valid spec → stored in DB

## Phase 5: Code Activity (Sequential First)
- [ ] 5.1 Code prompt template (`internal/agents/prompts/code.tmpl`)
- [ ] 5.2 `CodeActivity` — generates `map[string]string` files from Spec (+ DesignTokens if available)
- [ ] 5.3 Write files to MinIO at `runs/{run_id}/`
- [ ] 5.4 Persist `agent_steps` with file manifest + token counts
- [ ] 5.5 Handle `AppKind == "pwa"` — emit `manifest.json`, `sw.js`, registration
- [ ] 5.6 Enforce max bundle size (~2MB source)
- [ ] 5.7 Integration test: Plan → Code → valid Next.js project in MinIO

## Phase 6: Design Activity (Parallel with Code)
- [ ] 6.1 Design prompt template (`internal/agents/prompts/design.tmpl`)
- [ ] 6.2 `DesignActivity` — produces `DesignTokens` (colors, spacing, typography, radius) + CopyTone
- [ ] 6.3 Workflow: start Design + Code as Futures concurrently
- [ ] 6.4 Signal/timeout handoff: if Design finishes within 20s of Code start, re-invoke Code with tokens
- [ ] 6.5 Persist Design step; Code step references Design tokens
- [ ] 6.6 Integration test: parallel execution, token handoff works

## Phase 7: QA Activity + Retry Loop
- [ ] 7.1 QA prompt template (`internal/agents/prompts/qa.tmpl`) — for interpreting tool output
- [ ] 7.2 `QAActivity` — deterministic checks:
  - [ ] `npm install && next build` (or `next lint`)
  - [ ] Broken internal link crawl (against Spec pages)
  - [ ] Basic a11y (axe-core on static output)
  - [ ] PWA: manifest.json validation, SW registration + cache strategy
- [ ] 7.3 LLM interprets raw tool output → structured `QAIssue[]` (blocking/warning)
- [ ] 7.4 `Passed` = build success + zero blocking issues
- [ ] 7.5 Workflow retry loop: max 3 QA retries, feed issues back to Code
- [ ] 7.6 On max retries: status = `needs_review`, partial bundle still previewable
- [ ] 7.7 Persist every QA/Code retry as separate `agent_steps` (attempt tracking)

## Phase 8: Preview — Static Export (Decision: Static Export Only)
- [ ] 8.1 Implement `internal/sandbox.Provisioner` with `StaticExportProvisioner`
- [ ] 8.2 `Provision(ctx, runID, files)` → signed MinIO URL (TTL: 1h) for `runs/{run_id}/index.html`
- [ ] 8.3 `Teardown(ctx, runID)` — no-op (signed URLs expire automatically)
- [ ] 8.4 Wire into workflow: after QA pass, call Provision, store preview URL in `generation_runs.preview_url`
- [ ] 8.5 API `/preview` endpoint returns signed URL
- [ ] 8.6 Frontend: iframe loads preview URL, handles expiry gracefully
- [ ] 8.7 Code agent: enforce `output: 'export'` in `next.config.js` for all generated apps

## Phase 9: Deploy — Static Hosting
- [ ] 9.1 `Promote(ctx, appID, runID)` → copy `runs/{run_id}/` to `apps/{slug}/` in MinIO
- [ ] 9.2 Configure Caddy/CloudFront: `{slug}.appgent.app` → `apps/{slug}/` with SSL
- [ ] 9.3 `POST /apps/{id}/deploy` — calls Promote, creates `deployments` row with live URL
- [ ] 9.4 `GET /apps/{id}/deployments` — lists live deployments
- [ ] 9.5 Frontend: deploy button, shows live URL, status

## Phase 10: Vector DB + RAG Context
- [ ] 10.1 Seed `design_patterns` table with component/layout/copy examples
- [ ] 10.2 Embedding model selection + pgvector index tuning
- [ ] 10.3 Plan activity: query similar patterns, inject into prompt as `RAGContext`
- [ ] 10.4 Design activity: same RAG context for token generation
- [ ] 10.5 Background job: extract patterns from successful runs → add to library

## Phase 11: Polish & Observability
- [ ] 11.1 Run history UI — version picker, diff view (file-level)
- [ ] 11.2 Agent step timeline — expandable input/output JSON, token usage
- [ ] 11.3 Error states — friendly messages for auth, rate limits, sandbox failures
- [ ] 11.4 Structured JSON logs across API + worker (request IDs, correlation)
- [ ] 11.5 Temporal Web UI integration — workflow links from run detail
- [ ] 11.6 Rate limiting per user (workflow start throttle)
- [ ] 11.7 Usage tracking — `tokens_used` aggregation per user/app

---

## Open Questions (Blockers)
- [ ] **Sandbox runtime** — Freestyle VMs vs self-managed (must resolve before Phase 8)

## Notes
- Phases 0–3 can be done in parallel by different people
- Phases 4–7 are sequential (each builds on previous)
- Phase 8 unblocks preview UX; Phase 9 unblocks deploy
- Phase 10 is a quality multiplier, not required for MVP
- Phase 11 is ongoing polish