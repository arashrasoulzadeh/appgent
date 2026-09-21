# API Contract — Go API Server

REST + JSON. Base path `/api/v1`. Auth via signed JWT in an `HttpOnly` cookie
(`session`), issued by `POST /auth/login`. All endpoints below except
`/auth/login` and `/healthz` require a valid session cookie; missing/invalid
→ `401 {"error": "unauthorized"}`.

Errors always take the shape `{"error": string, "details"?: object}` with an
appropriate 4xx/5xx status.

## Auth

### `POST /auth/login`
```json
// request
{ "email": "admin", "password": "admin" }
// response 200
{ "user": { "id": "uuid", "email": "admin" } }
// sets Set-Cookie: session=<jwt>; HttpOnly; SameSite=Lax; Secure (in prod)
```
401 on bad credentials: `{"error": "invalid credentials"}`.

### `POST /auth/logout`
Clears the session cookie. `204 No Content`.

### `GET /auth/me`
```json
// response 200
{ "user": { "id": "uuid", "email": "admin" } }
```

## Apps

### `GET /apps`
List the current user's apps.
```json
// response 200
{ "apps": [
  { "id": "uuid", "name": "My Portfolio", "slug": "my-portfolio-a1b2",
    "kind": "website", "status": "ready", "created_at": "..." }
] }
```

### `POST /apps`
Create an app and kick off the first generation run in one call.
```json
// request
{ "name": "My Portfolio", "kind": "website", "prompt": "A portfolio site for a photographer, dark theme, gallery + contact form" }
// response 201
{ "app": { "id": "uuid", "name": "My Portfolio", "slug": "my-portfolio-a1b2", "kind": "website", "status": "generating" },
  "run": { "id": "uuid", "version": 1, "status": "queued" } }
```
Server-side: inserts `apps` row, inserts `generation_runs` version 1, starts
the `GenerateAppWorkflow` Temporal workflow, stores `temporal_workflow_id`.

### `GET /apps/{app_id}`
```json
// response 200
{ "app": { "...": "as above" },
  "latest_run": { "id": "uuid", "version": 3, "status": "ready" } }
```
404 if the app doesn't belong to the caller (never leak existence of other
users' apps — respond 404, not 403).

### `DELETE /apps/{app_id}`
Soft-delete not required for MVP — hard delete cascades to runs/steps/deployments
per the FK `ON DELETE CASCADE`. `204 No Content`.

### `POST /apps/{app_id}/regenerate`
Create a new `generation_runs` version from a new prompt (or empty prompt to
re-run the last one), start a new workflow.
```json
// request
{ "prompt": "Same as before but add a blog section" }
// response 202
{ "run": { "id": "uuid", "version": 4, "status": "queued" } }
```

## Runs

### `GET /apps/{app_id}/runs`
List all versions for the run-history UI.
```json
{ "runs": [
  { "id": "uuid", "version": 4, "status": "running", "user_prompt": "...", "created_at": "..." },
  { "id": "uuid", "version": 3, "status": "ready", "user_prompt": "...", "created_at": "..." }
] }
```

### `GET /apps/{app_id}/runs/{run_id}`
Full run detail including every agent step, for the timeline UI.
```json
{ "run": { "id": "uuid", "version": 3, "status": "ready", "error": null },
  "steps": [
    { "agent_type": "plan", "attempt": 1, "status": "succeeded", "model_used": "nvidia/nemotron-3-ultra-550b-a55b:free",
      "started_at": "...", "finished_at": "...", "output_summary": "5 pages, 12 components planned" },
    { "agent_type": "design", "attempt": 1, "status": "succeeded", "...": "..." },
    { "agent_type": "code", "attempt": 1, "status": "succeeded", "...": "..." },
    { "agent_type": "qa", "attempt": 1, "status": "failed", "output_summary": "2 broken links, missing alt text on 3 images" },
    { "agent_type": "code", "attempt": 2, "status": "succeeded", "...": "..." },
    { "agent_type": "qa", "attempt": 2, "status": "succeeded", "...": "..." }
  ] }
```
`output` full JSON is available but the list endpoint returns a truncated
`output_summary` string; add a `GET /apps/{app_id}/runs/{run_id}/steps/{step_id}`
if the UI needs the raw payload (e.g. to show full generated file diffs).

### `GET /apps/{app_id}/runs/{run_id}/events` (SSE)
Live progress stream while a run is `queued`/`running`. Content-Type
`text/event-stream`. Each event's `data` is a JSON-encoded step update or
terminal run status:
```
event: step
data: {"agent_type":"code","status":"running"}

event: step
data: {"agent_type":"code","status":"succeeded"}

event: run
data: {"status":"ready"}
```
Server implementation: the API handler polls the Temporal workflow via
`QueryWorkflow` (see [temporal-workflow.md](temporal-workflow.md#queries)) on
a short interval (e.g. 1s) and forwards diffs, or subscribes to a Postgres
`LISTEN/NOTIFY` channel that activities `NOTIFY` on when they update
`agent_steps` — prefer LISTEN/NOTIFY to avoid polling Temporal from every
open SSE connection.

## Deployments

### `POST /apps/{app_id}/deploy`
Promote the latest successful run's bundle to persistent hosting.
```json
// response 202
{ "deployment": { "id": "uuid", "status": "deploying" } }
```

### `GET /apps/{app_id}/deployments`
```json
{ "deployments": [
  { "id": "uuid", "url": "https://my-portfolio-a1b2.appgent.app", "status": "live", "deployed_at": "..." }
] }
```

## Preview

### `GET /apps/{app_id}/runs/{run_id}/preview`
Returns the short-lived sandbox preview URL for a specific run version (not
necessarily the latest — lets the run-history UI preview any past version).
```json
{ "preview_url": "https://preview-<run_id>.sandbox.appgent.app", "expires_at": "..." }
```
Implementation depends on the sandbox runtime decision — see
[open-questions.md](open-questions.md).

## Health

### `GET /healthz`
No auth. `200 {"status":"ok"}` — checks DB connectivity, not Temporal
(keep it cheap; used by container orchestration liveness probes).
