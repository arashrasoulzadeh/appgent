# Temporal Workflow & Activity Contracts

Package layout: workflow/activity definitions live in `internal/temporal/`;
the OpenRouter-calling logic they invoke lives in `internal/agents/` (kept
separate so activities stay thin and the agent logic is unit-testable
without a Temporal test environment).

## Workflow: `GenerateAppWorkflow`

```go
// internal/temporal/workflow.go

type GenerateAppInput struct {
    RunID      uuid.UUID
    AppID      uuid.UUID
    AppKind    string // "website" | "pwa"
    UserPrompt string
}

type GenerateAppResult struct {
    Status string // "succeeded" | "needs_review" | "failed"
    Error  string
}

func GenerateAppWorkflow(ctx workflow.Context, in GenerateAppInput) (GenerateAppResult, error)
```

Task queue: `"appgent-generation"`. Workflow ID: `"run-" + RunID.String()`
(deterministic — lets the API server re-derive it, and prevents accidental
double-starts for the same run).

### Execution graph

1. **Plan** — `ExecuteActivity(ctx, PlanActivity, PlanInput{...})`, blocking.
   Produces `Spec` (pages, components, data model, style direction).
2. **Design ∥ Code**, started concurrently as `workflow.Future`s:
   - `designFuture := workflow.ExecuteActivity(ctx, DesignActivity, DesignInput{Spec: spec})`
   - `codeFuture := workflow.ExecuteActivity(ctx, CodeActivity, CodeInput{Spec: spec, DesignTokens: nil})`
   - The Code activity is started immediately with `DesignTokens: nil` (it
     falls back to sane defaults). When Design finishes, the workflow sends
     a **Signal** (`"design-tokens-ready"`) carrying the tokens to the
     *same* Code activity run if it supports mid-flight signals via a
     `workflow.Channel`, **or** — simpler and recommended for v1 — the
     workflow just `workflow.Await`s both futures and re-invokes Code with
     final tokens only if Design finished within a short timeout (e.g. 20s)
     of Code starting; otherwise Code's default-token output stands. Pick
     the "re-invoke if fast enough" approach for v1; true mid-activity
     signalling is unnecessary complexity until profiling shows it matters.
3. **QA loop** (bounded, `maxQARetries = 3`):
   ```
   for attempt := 1; attempt <= maxQARetries; attempt++ {
       qaResult := ExecuteActivity(QAActivity, QAInput{Files: codeOutput.Files, Spec: spec})
       if qaResult.Passed { break }
       if attempt == maxQARetries { status = "needs_review"; break }
       codeOutput = ExecuteActivity(CodeActivity, CodeInput{
           Spec: spec, DesignTokens: designOutput.Tokens,
           PriorFiles: codeOutput.Files, QAFeedback: qaResult.Issues,
       })
   }
   ```
4. Workflow returns `GenerateAppResult{Status: ..., Error: ...}`. The API
   server (or a final activity, `PersistRunResultActivity`) writes the
   terminal `generation_runs.status` and `bundle_path`.

### Retry policy

- Each `ExecuteActivity` call uses `workflow.ActivityOptions{ StartToCloseTimeout: 5*time.Minute, RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: 3, InitialInterval: 5*time.Second} }`
  for transient failures (OpenRouter 5xx/timeout). This is Temporal-level
  retry for *infrastructure* failures — separate from the QA-driven
  *content* retry loop in step 3 above.
- `StartToCloseTimeout` may need to be longer for `CodeActivity` (larger
  generations) — tune independently per activity, don't share one
  `ActivityOptions` struct if timeouts diverge.

### Queries

Expose a Query handler so the API's SSE endpoint (or a polling fallback) can
read live status without waiting for the workflow to complete:

```go
workflow.SetQueryHandler(ctx, "status", func() (WorkflowStatus, error) {
    return currentStatus, nil // currentStatus updated after each activity completes
})
```

Prefer Postgres `LISTEN/NOTIFY` from within activities (see
[api.md](api.md#get-appsapp_idrunsrun_idevents-sse)) as the primary
mechanism for the SSE endpoint — it decouples the API server from having an
open Temporal client query per connected browser tab. Use the Query handler
as a fallback/debugging tool (also visible directly in the Temporal Web UI).

## Activities

All activity inputs/outputs are JSON-serializable structs; the same structs
back the `agent_steps.input`/`output` JSONB columns (an activity, on
completion, persists its own `agent_steps` row — don't make the workflow do
DB writes directly, activities have retry semantics that fit "write once on
success").

### `PlanActivity(ctx, PlanInput) (PlanOutput, error)`
```go
type PlanInput struct {
    AppKind    string
    UserPrompt string
    RAGContext []DesignPattern // from pgvector similarity search, may be empty
}
type PlanOutput struct {
    Pages      []PageSpec
    Components []ComponentSpec
    DataModel  []EntitySpec // for apps that need client-side state/forms
    StyleDirection string    // free-text brief handed to Design
}
```

### `DesignActivity(ctx, DesignInput) (DesignOutput, error)`
```go
type DesignInput struct {
    Spec PlanOutput
}
type DesignOutput struct {
    Tokens    DesignTokens // colors, spacing scale, typography, radius
    CopyTone  string       // voice/tone guide for microcopy
    LayoutNotes string
}
```

### `CodeActivity(ctx, CodeInput) (CodeOutput, error)`
```go
type CodeInput struct {
    Spec         PlanOutput
    DesignTokens *DesignTokens // nil on first pass before Design finishes
    PriorFiles   map[string]string // path -> content, set on QA-retry passes
    QAFeedback   []QAIssue         // set on QA-retry passes
    AppKind      string            // "website" | "pwa" — drives manifest/SW generation
}
type CodeOutput struct {
    Files map[string]string // path -> content, written to object storage by the activity
}
```

### `QAActivity(ctx, QAInput) (QAOutput, error)`
```go
type QAInput struct {
    Files   map[string]string
    Spec    PlanOutput
    AppKind string
}
type QAOutput struct {
    Passed bool
    Issues []QAIssue // {File, Line, Severity, Message}
}
```
Checks run in order: build/compile check (actually run `next build` or
equivalent in a throwaway sandbox/container — not just LLM judgment),
lint, broken internal links, basic a11y (alt text, label associations), and
— when `AppKind == "pwa"` — manifest.json validity and service worker
registration/offline-strategy presence. The LLM call is used to interpret
build/lint output and produce human-readable `Issues`, not to "guess"
whether code is correct where a deterministic tool can check it. Prefer
real tooling (tsc/eslint/next build) over LLM judgment wherever possible.

## Worker configuration

`services/worker/main.go` starts a `worker.New(client, "appgent-generation", worker.Options{})`,
registers the workflow and all four activities plus `PersistRunResultActivity`.
Concurrency (per §7 decision — relying on Temporal's built-in limits):
set `worker.Options{MaxConcurrentActivityExecutionSize: N}` and/or a
per-user rate limit via a `Session`/dedicated task queue per user if a
single global limit proves too coarse later. Start with a single global
limit for v1.
