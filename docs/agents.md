# Agent Prompt Specs

All four agents call OpenRouter using model `nvidia/nemotron-3-ultra:free`
(configurable via `OPENROUTER_MODEL_*` env vars per agent — see
[environment.md](environment.md) — so any agent can be swapped to a stronger
paid model later without a code change). All calls request structured JSON
output (OpenRouter/most models support a `response_format: {type: "json_object"}`
or JSON-schema constrained mode — use it; do not hand-parse free text).

Each agent's system prompt should be a template in
`internal/agents/prompts/{plan,design,code,qa}.tmpl` (Go `text/template`),
rendered with the structs from [temporal-workflow.md](temporal-workflow.md).
Keep prompts in template files, not inline Go strings — makes iteration and
diffing prompt changes trivial.

## Plan Agent

**Role**: turn a free-text user prompt into a concrete build spec.

**System prompt guidance**:
- State the app kind (`website` or `pwa`) and that PWA means a *real*
  offline-capable app (manifest + service worker + explicit caching
  strategy), not a cosmetic flag — this drives what pages/data the agent
  should design for offline use.
- Output must be valid JSON matching `PlanOutput` (pages, components, data
  model, style direction). Keep `Pages` to a reasonable count for a single
  generation (cap suggestion: 3–8 pages) to bound Code agent output size.
- If `RAGContext` (design patterns from pgvector) is non-empty, instruct the
  model to prefer proven patterns over inventing new structure; if empty,
  proceed with sensible defaults — never say "no context provided" in a way
  that degrades output quality.

## Design Agent

**Role**: produce a cohesive visual system from the Plan spec.

**System prompt guidance**:
- Output `DesignTokens` (color palette incl. dark-mode variants, spacing
  scale, typography stack, border radius scale) plus a copy-tone brief.
- Ground choices in the Plan's `StyleDirection` field, don't re-derive from
  the raw user prompt (keeps Design and Code consistent since both read the
  same upstream Plan output).
- Tokens should map directly onto Tailwind CSS config (`tailwind.config` theme
  extension shape) or CSS custom properties — pick one and document it in
  the template so Code agent output can consume it mechanically, not via
  another LLM interpretation step.

## Code Agent

**Role**: generate the actual Next.js site/PWA source files.

**System prompt guidance**:
- Stack: Next.js (App Router), TypeScript, Tailwind CSS. Static-exportable
  where possible (`output: 'export'`) unless the Plan spec requires
  server-side logic — keep generated apps as simple/static as the spec
  allows, since simpler bundles are easier to sandbox/preview/deploy.
- On first pass (`PriorFiles` empty): generate a complete file set from
  `Spec` + `DesignTokens` (may be `nil` — use sane token defaults inline).
- On retry pass (`PriorFiles` + `QAFeedback` set): the prompt must include
  the full prior file set and the QA issues list, and instruct the model to
  return the **complete updated file set** (not a diff) — simpler for the
  activity to persist, avoids patch-application bugs. Explicitly tell the
  model to fix only what QA flagged and otherwise preserve prior output, to
  avoid unnecessary full-app rewrites each retry.
- When `AppKind == "pwa"`: must emit `public/manifest.json` and a service
  worker (e.g. `public/sw.js` + registration in the root layout) implementing
  a real cache strategy per route (static assets cache-first, dynamic data
  network-first with cache fallback) — this is a hard requirement, not
  optional, per the "full offline-capable PWA" scope decision.
- Output is `map[string]string` of relative file path → full file content.
  Enforce a max total size (e.g. reject/truncate generations over ~2MB of
  source) to keep sandbox provisioning fast.

## QA Agent

**Role**: hybrid — real tooling first, LLM to interpret/summarize.

**Process** (the `QAActivity`, not pure prompting):
1. Write `Files` to a throwaway working directory (or reuse the sandbox
   provisioning path once the runtime is chosen — see
   [open-questions.md](open-questions.md)).
2. Run deterministic checks: `npm install && next build` (or `next lint`),
   a broken-internal-link check (simple crawl of generated `<a href>` /
   `<Link href>` against the known page list from `Spec`), an a11y pass
   (e.g. `axe-core` against the built static output), and — for
   `AppKind == "pwa"` — validate `manifest.json` against the W3C schema and
   confirm the service worker registers and defines at least one cache
   strategy.
3. Feed the raw tool output (build errors, lint errors, a11y violations,
   manifest validation errors) to the LLM **only** to turn it into a
   structured, deduplicated `QAIssue` list with clear `File`/`Line`/
   `Message` — the LLM does not decide pass/fail; `Passed` is computed
   deterministically as "build succeeded AND zero blocking-severity issues".
4. Severity levels: `blocking` (build failure, missing manifest for PWA,
   broken internal link) vs `warning` (a11y nits, lint style issues).
   Only `blocking` issues fail the run / trigger a Code retry; `warning`
   issues are surfaced in the UI but don't block preview/deploy.

This keeps QA fast, deterministic where it matters, and avoids the failure
mode of an LLM confidently approving broken code.
