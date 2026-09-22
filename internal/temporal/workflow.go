package temporal

import (
	"fmt"
	"regexp"
	"sort"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// componentImportRe matches `from "@/components/Name"` (or single quotes) —
// used by findMissingComponentImports to catch a real, recurring class of
// build failure: a page/component file imports a component name the LLM
// invented, that was never actually in the Plan spec's component list and
// so was never generated as its own file. That's structurally different
// from "generation of an existing target failed" (which the failure-
// isolation logic in generateCode already handles) — this is catching an
// import that was never going to exist in the first place, no matter how
// many times the SAME target is retried.
var componentImportRe = regexp.MustCompile(`from\s+["']@/components/([A-Za-z0-9_]+)["']`)

// findMissingComponentImports scans generated files for
// `@/components/<Name>` imports and returns the names that have no
// corresponding src/components/<Name>.tsx file among files. Deterministic
// (sorted) since this runs directly in workflow code.
func findMissingComponentImports(files map[string]string) []string {
	byFile := findMissingComponentImportsByFile(files)
	seen := map[string]bool{}
	var missing []string
	for _, names := range byFile {
		for _, name := range names {
			if !seen[name] {
				seen[name] = true
				missing = append(missing, name)
			}
		}
	}
	sort.Strings(missing)
	return missing
}

// findMissingComponentImportsByFile is like findMissingComponentImports but
// keyed by which file(s) actually contain each bad import — needed so a
// synthesized QAIssue can set File correctly. code.tmpl's retry-pass rule
// tells each per-target call "if QA feedback doesn't mention any file in
// your scope, return your scope's files unchanged" — a QAIssue with no File
// attribution would be ignored by every target, including the one that
// actually needs to fix it, since none of them would recognize the
// feedback as being about their own file.
func findMissingComponentImportsByFile(files map[string]string) map[string][]string {
	byFile := map[string][]string{}
	for path, content := range files {
		seen := map[string]bool{}
		var names []string
		for _, m := range componentImportRe.FindAllStringSubmatch(content, -1) {
			name := m[1]
			if seen[name] {
				continue
			}
			if _, ok := files["src/components/"+name+".tsx"]; ok {
				continue
			}
			seen[name] = true
			names = append(names, name)
		}
		if len(names) > 0 {
			sort.Strings(names)
			byFile[path] = names
		}
	}
	return byFile
}

type GenerateAppInput struct {
	RunID      uuid.UUID
	AppID      uuid.UUID
	AppKind    string
	UserPrompt string
	// ParallelCode controls whether code-generation targets (pages,
	// components, shared/root files) run concurrently (rolling window of
	// maxParallelCode) or strictly one at a time. Defaults to true (the
	// zero value is false, so callers must set it explicitly — see
	// AppService.Create/Regenerate, which default it from an env var).
	ParallelCode bool
}

type GenerateAppResult struct {
	Status string
	Error  string
	// BundlePath is the object-storage prefix (e.g. "runs/<runID>/") the
	// generated files were published under, set on the "succeeded" and
	// "needs_review" exit paths whenever publishing succeeded. Empty means
	// no preview/deploy is available for this run (either it never
	// produced files, or publishing itself failed — which is logged but
	// deliberately non-fatal to the run's own status).
	BundlePath string
	// SourcePath is the object-storage prefix the run's raw generated
	// source was saved under, independent of whether the build itself
	// succeeded — this is what lets Deploy retry a failed build later
	// (RedeployWorkflow) without a full regenerate.
	SourcePath string
}

// PublishBundleInput/Output are used by PublishBundleActivity, which
// uploads a run's final generated files to object storage so they can be
// served back for preview/deployment. See internal/agents.PublishBundleActivity.
type PublishBundleInput struct {
	RunID uuid.UUID
	Files map[string]string
}

type PublishBundleOutput struct {
	BundlePath string
}

// PublishSourceInput/Output are used by PublishSourceActivity, which
// durably saves a run's raw generated source (before build) — separately
// from PublishBundleActivity's built output — so a failed build can be
// retried later (RedeployWorkflow) without a full regenerate.
type PublishSourceInput struct {
	RunID uuid.UUID
	Files map[string]string
}

type PublishSourceOutput struct {
	SourcePath string
}

// RedeployInput/Result drive RedeployWorkflow — triggered by the Deploy
// button. NeedsBuild is decided by AppService.Deploy before starting the
// workflow: false when the target run already has a built bundle (just
// promote it, fast), true when only raw source was saved (a previous
// build failed, or this is the first deploy attempt for that run) and the
// build must be retried first, without a full regenerate.
type RedeployInput struct {
	AppID        uuid.UUID
	RunID        uuid.UUID
	DeploymentID uuid.UUID
	NeedsBuild   bool
}

type RedeployResult struct {
	Status string // "live" or "failed"
	Error  string
}

type FetchRunSourceInput struct {
	RunID uuid.UUID
}

type FetchRunSourceOutput struct {
	Files map[string]string
}

type UpdateRunBundlePathInput struct {
	RunID      uuid.UUID
	BundlePath string
}

type PromoteDeploymentInput struct {
	AppID        uuid.UUID
	RunID        uuid.UUID
	DeploymentID uuid.UUID
}

type PromoteDeploymentOutput struct {
	URL string
}

type MarkDeploymentFailedInput struct {
	DeploymentID uuid.UUID
	Error        string
}

type PlanInput struct {
	RunID      uuid.UUID
	AppKind    string
	UserPrompt string
	RAGContext []DesignPattern
}

type PlanOutput struct {
	Pages          []PageSpec
	Components     []ComponentSpec
	DataModel      []EntitySpec
	StyleDirection string
}

type PageSpec struct {
	Name        string
	Path        string
	Description string
}

type ComponentSpec struct {
	Name        string
	Type        string
	Description string
	Props       map[string]interface{}
}

type EntitySpec struct {
	Name   string
	Fields []FieldSpec
}

type FieldSpec struct {
	Name string
	Type string
}

type DesignPattern struct {
	Kind    string
	Title   string
	Content string
}

type DesignInput struct {
	RunID uuid.UUID
	Spec  PlanOutput
}

type DesignOutput struct {
	Tokens      DesignTokens
	CopyTone    string
	LayoutNotes string
}

type DesignTokens struct {
	Colors       ColorPalette
	Spacing      []string
	Typography   Typography
	BorderRadius []string
}

type ColorPalette struct {
	Primary   ColorVariant
	Secondary ColorVariant
	Accent    ColorVariant
	Neutral   ColorVariant
	Success   ColorVariant
	Warning   ColorVariant
	Error     ColorVariant
}

type ColorVariant struct {
	Light    string
	Main     string
	Dark     string
	Contrast string
}

type Typography struct {
	FontFamily  string
	FontSizes   map[string]string
	FontWeights map[string]int
	LineHeights map[string]string
}

type CodeInput struct {
	RunID        uuid.UUID
	Attempt      int
	Spec         PlanOutput
	DesignTokens *DesignTokens
	PriorFiles   map[string]string
	QAFeedback   []QAIssue
	AppKind      string
	// TargetPage and TargetComponent scope this call. At most one is set:
	//   - TargetPage set: generate just that one page's route file (plus
	//     any small page-local components used only by it).
	//   - TargetComponent set: generate just that one shared/layout
	//     component (e.g. Header, Footer) — components with
	//     ComponentSpec.Type == "layout" in the Plan spec, since those are
	//     the ones referenced across multiple pages and worth generating
	//     once, independently, rather than duplicated or coordinated
	//     page-by-page.
	//   - Both nil: generate the shared/root files (layout.tsx, package.json,
	//     tsconfig.json, next.config.js, globals.css, manifest/sw for pwa)
	//     only — no components, no pages.
	// Splitting work this way lets pages and shared components generate as
	// parallel Temporal activities instead of one huge single-shot call.
	TargetPage      *PageSpec
	TargetComponent *ComponentSpec
}

type CodeOutput struct {
	Files map[string]string
}

type QAInput struct {
	RunID   uuid.UUID
	Attempt int
	Files   map[string]string
	Spec    PlanOutput
	AppKind string
}

type QAOutput struct {
	Passed bool
	Issues []QAIssue
}

type QAIssue struct {
	File     string
	Line     int
	Severity string
	Message  string
}

const (
	planActivityName          = "PlanActivity"
	designActivityName        = "DesignActivity"
	codeActivityName          = "CodeActivity"
	qaActivityName            = "QAActivity"
	persistActivityName       = "PersistRunResultActivity"
	publishActivityName       = "PublishBundleActivity"
	publishSourceActivityName = "PublishSourceActivity"

	fetchRunSourceActivityName       = "FetchRunSourceActivity"
	updateRunBundlePathActivityName  = "UpdateRunBundlePathActivity"
	promoteDeploymentActivityName    = "PromoteDeploymentActivity"
	markDeploymentFailedActivityName = "MarkDeploymentFailedActivity"
)

// GenerateAppWorkflow orchestrates Plan -> (Design || Code) -> QA-retry-loop.
// Activities are referenced by name (not by local function value) because
// their real implementations live in internal/agents, registered on the
// worker under these same names — see services/worker/cmd/worker/main.go.
func GenerateAppWorkflow(ctx workflow.Context, in GenerateAppInput) (result GenerateAppResult, err error) {
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
			InitialInterval: 5 * time.Second,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// CodeActivity generates one page/shared-file-set per call and can be
	// the slowest step when the underlying model is a slow/overloaded
	// free-tier one — give it its own longer timeout rather than sharing
	// the 5-minute default with the lighter Plan/Design/QA activities.
	// The AI provider HTTP client itself times out at 240s (internal/ai);
	// with up to 3 retries that's up to 12 minutes, so this needs real
	// headroom above that, not just above one attempt.
	codeCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 15 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
			InitialInterval: 5 * time.Second,
		},
	})
	codeAttempt := 0

	// PublishBundleActivity now runs `npm install && npm run build` in an
	// ephemeral container before uploading — much slower than a plain
	// object-storage upload, so this needs real headroom above the
	// Builder's own internal 10-minute timeout. A build failure from bad
	// generated code fails identically on every attempt, so retries exist
	// only for transient infra blips (npm registry, docker daemon) — 2 is
	// plenty, unlike a 3-attempt budget that'd just triple the wasted time
	// on a genuinely broken build.
	publishCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 15 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 2,
			InitialInterval: 5 * time.Second,
		},
	})

	// Persist whatever the final result ends up being, on every exit path
	// (success, needs_review, or failure) — this is what keeps
	// generation_runs.status / apps.status in sync with what actually
	// happened. Runs on a disconnected context so it still executes even
	// if ctx itself was cancelled.
	defer func() {
		persistCtx, cancel := workflow.NewDisconnectedContext(ctx)
		defer cancel()
		persistCtx = workflow.WithActivityOptions(persistCtx, ao)
		persistErr := workflow.ExecuteActivity(persistCtx, persistActivityName, in.RunID, in.AppID, result).Get(persistCtx, nil)
		if persistErr != nil && err == nil {
			err = persistErr
		}
	}()

	var planOutput PlanOutput
	planErr := workflow.ExecuteActivity(ctx, planActivityName, PlanInput{
		RunID:      in.RunID,
		AppKind:    in.AppKind,
		UserPrompt: in.UserPrompt,
		RAGContext: []DesignPattern{},
	}).Get(ctx, &planOutput)
	if planErr != nil {
		result = GenerateAppResult{Status: "failed", Error: planErr.Error()}
		return result, planErr
	}

	// generateCode fans code generation out into one call per page, one per
	// component (ALL of them — layout, ui, form, section, whatever — each
	// generated exactly once by its own dedicated call, never regenerated
	// redundantly by multiple pages), plus one for the shared/root files
	// (layout.tsx, config, globals.css, manifest/sw for pwa). This replaces
	// a single call that generated the whole app's file tree in one LLM
	// response — smaller, focused calls are both faster (real concurrency)
	// and safer (a single call producing truncated/invalid output only
	// loses that one piece, not the entire run).
	//
	// Concurrency is a rolling window of maxParallelCode, not
	// batch-of-5-then-wait-for-all-5: as soon as any one call finishes, the
	// next queued target starts immediately, rather than idling finished
	// slots until the slowest call in the current batch finishes. This
	// matters in practice — a free-tier model's per-call latency varies
	// wildly (seen in production: anywhere from ~30s to several minutes),
	// so a strict batch-wait-batch scheme means one slow call in a batch of
	// 5 stalls the other 4 already-idle slots instead of picking up new
	// work.
	//
	// Failure isolation: when one target's CodeActivity call fails (after
	// exhausting codeCtx's own Temporal-level RetryPolicy), only THAT
	// target is retried (up to maxTargetRetries extra full attempts) — the
	// other in-flight/queued targets are never touched or re-run. Only a
	// PAGE target still failing after its retries is skipped and treated
	// as non-fatal: nothing else references a specific page, so a missing
	// route just degrades gracefully. Shared/root files AND components are
	// both fatal on failure — other already-generated files (layout.tsx,
	// pages) directly `import` components by name, so a missing component
	// guarantees a build failure ("Module not found") rather than
	// degrading gracefully, exactly like a missing layout.tsx/package.json
	// would.
	maxParallelCode := 5
	if !in.ParallelCode {
		maxParallelCode = 1
	}
	const maxTargetRetries = 1
	type codeTarget struct {
		page      *PageSpec
		component *ComponentSpec
	}
	targetLabel := func(t codeTarget) string {
		switch {
		case t.page != nil:
			return "page:" + t.page.Path
		case t.component != nil:
			return "component:" + t.component.Name
		default:
			return "shared/root"
		}
	}
	generateCode := func(tokens *DesignTokens, priorFiles map[string]string, qaFeedback []QAIssue) (CodeOutput, error) {
		codeAttempt++
		attempt := codeAttempt

		targets := make([]codeTarget, 0, len(planOutput.Pages)+len(planOutput.Components)+1)
		targets = append(targets, codeTarget{}) // shared/root files
		for i := range planOutput.Components {
			c := planOutput.Components[i]
			targets = append(targets, codeTarget{component: &c})
		}
		for i := range planOutput.Pages {
			page := planOutput.Pages[i]
			targets = append(targets, codeTarget{page: &page})
		}

		merged := make(map[string]string)
		var failedTargets []string
		var firstErr error
		targetRetries := make([]int, len(targets))
		nextIdx := 0
		inFlight := make(map[workflow.Future]int)

		launchTarget := func(idx int) {
			target := targets[idx]
			f := workflow.ExecuteActivity(codeCtx, codeActivityName, CodeInput{
				RunID:           in.RunID,
				Attempt:         attempt,
				Spec:            planOutput,
				DesignTokens:    tokens,
				PriorFiles:      priorFiles,
				QAFeedback:      qaFeedback,
				AppKind:         in.AppKind,
				TargetPage:      target.page,
				TargetComponent: target.component,
			})
			inFlight[f] = idx
		}
		launchNext := func() {
			if nextIdx >= len(targets) {
				return
			}
			idx := nextIdx
			nextIdx++
			launchTarget(idx)
		}

		for i := 0; i < maxParallelCode && i < len(targets); i++ {
			launchNext()
		}

		for len(inFlight) > 0 {
			selector := workflow.NewSelector(ctx)
			for f, idx := range inFlight {
				f, idx := f, idx
				selector.AddFuture(f, func(f workflow.Future) {
					delete(inFlight, f)
					var out CodeOutput
					if err := f.Get(ctx, &out); err != nil {
						if targetRetries[idx] < maxTargetRetries {
							targetRetries[idx]++
							launchTarget(idx)
							return
						}
						target := targets[idx]
						if target.page == nil {
							// Shared/root files AND components are
							// load-bearing for the rest of the app: page/
							// layout files generated by OTHER targets
							// already assume `@/components/<Name>` exists
							// and import it directly, so a missing
							// component doesn't degrade gracefully — it
							// guarantees a build failure ("Module not
							// found") for every file that imports it. Only
							// an actual PAGE is safe to skip (that route
							// just won't exist; nothing else references
							// it). Other already in-flight targets are
							// still allowed to finish/drain below; we just
							// stop queuing new work.
							if firstErr == nil {
								firstErr = err
							}
							return
						}
						// A page target exhausted its retries — skip it,
						// keep the rest of the run going. Nothing else in
						// the generated app references a specific page by
						// import, so a missing route degrades gracefully
						// instead of breaking the build.
						failedTargets = append(failedTargets, targetLabel(target))
						launchNext()
						return
					}
					for path, content := range out.Files {
						merged[path] = content
					}
					if firstErr == nil {
						launchNext()
					}
				})
			}
			selector.Select(ctx)
		}

		if firstErr != nil {
			return CodeOutput{}, firstErr
		}
		if len(failedTargets) > 0 {
			workflow.GetLogger(ctx).Warn("code generation: some targets failed after retries and were skipped",
				"runID", in.RunID, "attempt", attempt, "failedTargets", failedTargets)
		}
		return CodeOutput{Files: merged}, nil
	}

	// Design runs concurrently with the (fanned-out, parallel-by-page) Code
	// generation below — the design Future is dispatched here and only
	// awaited after generateCode's own futures have been issued.
	designFuture := workflow.ExecuteActivity(ctx, designActivityName, DesignInput{RunID: in.RunID, Spec: planOutput})

	codeOutput, codeErr := generateCode(nil, nil, nil)
	if codeErr != nil {
		result = GenerateAppResult{Status: "failed", Error: codeErr.Error()}
		return result, codeErr
	}

	// finishWithBundle is the single exit point for the "succeeded" and
	// "needs_review" statuses — both have real generated files worth
	// previewing/deploying, unlike "failed". Publishing failure is logged
	// but never turns an otherwise-successful generation into a failed
	// run; it just leaves BundlePath empty, so preview/deploy are
	// unavailable for that run until it's regenerated.
	finishWithBundle := func(status, errMsg string) (GenerateAppResult, error) {
		bundlePath := ""
		sourcePath := ""
		if len(codeOutput.Files) > 0 {
			// Last-resort backstop: the self-heal retry loop above gives
			// the LLM up to maxQARetries chances to stop importing a
			// component it never actually generated, but nothing
			// guarantees it succeeds — it can regenerate the exact same
			// hallucinated import every attempt. Rather than let a
			// guaranteed-to-fail "Module not found" build error reach the
			// user after all retries are already spent, stub out any
			// components still missing at this point so the build can
			// always at least SUCCEED (with that one piece silently
			// blank) instead of hard-failing on something retries already
			// tried and failed to fix.
			for _, name := range findMissingComponentImports(codeOutput.Files) {
				codeOutput.Files["src/components/"+name+".tsx"] = fmt.Sprintf(
					"// Auto-generated placeholder: the code-generation model referenced\n"+
						"// this component without ever defining it, and retries didn't fix it.\n"+
						"function %sPlaceholder() {\n  return null\n}\nexport default %sPlaceholder\nexport { %sPlaceholder as %s }\n",
					name, name, name, name,
				)
			}

			// Source is saved FIRST and independently of the build, so it
			// survives even when the build itself fails below — that's
			// what lets Deploy retry the build later (RedeployWorkflow)
			// without a full regenerate.
			var srcOut PublishSourceOutput
			srcErr := workflow.ExecuteActivity(publishCtx, publishSourceActivityName, PublishSourceInput{
				RunID: in.RunID,
				Files: codeOutput.Files,
			}).Get(publishCtx, &srcOut)
			if srcErr != nil {
				workflow.GetLogger(ctx).Warn("publish source failed", "runID", in.RunID, "error", srcErr)
			} else {
				sourcePath = srcOut.SourcePath
			}

			var pubOut PublishBundleOutput
			pubErr := workflow.ExecuteActivity(publishCtx, publishActivityName, PublishBundleInput{
				RunID: in.RunID,
				Files: codeOutput.Files,
			}).Get(publishCtx, &pubOut)
			if pubErr != nil {
				workflow.GetLogger(ctx).Warn("publish bundle failed", "runID", in.RunID, "error", pubErr)
			} else {
				bundlePath = pubOut.BundlePath
			}
		}
		result = GenerateAppResult{Status: status, Error: errMsg, BundlePath: bundlePath, SourcePath: sourcePath}
		return result, nil
	}

	var designOutput DesignOutput
	if err := designFuture.Get(ctx, &designOutput); err != nil {
		result = GenerateAppResult{Status: "failed", Error: err.Error()}
		return result, err
	}

	// If design finished, re-run Code with design tokens
	if designOutput.Tokens.Colors != (ColorPalette{}) {
		updatedCodeOutput, execErr := generateCode(&designOutput.Tokens, codeOutput.Files, nil)
		if execErr != nil {
			result = GenerateAppResult{Status: "failed", Error: execErr.Error()}
			return result, execErr
		}
		codeOutput = updatedCodeOutput
	}

	// QA loop
	maxQARetries := 3
	for attempt := 1; attempt <= maxQARetries; attempt++ {
		var qaOutput QAOutput

		// Cheap, deterministic, zero-LLM-token structural check before
		// spending a QA call: a page/component importing
		// "@/components/X" where X was never actually generated (the LLM
		// invented a component name that was never in the Plan spec's
		// component list) is a guaranteed build failure — "Module not
		// found: Can't resolve '@/components/X'" — no matter how many
		// times the SAME set of targets gets regenerated by luck alone.
		// Catching it here, synthesizing the same QAIssue shape a real QA
		// pass would produce, and feeding it into the SAME retry-with-
		// feedback path below means this self-heals within one run's
		// existing retry budget instead of requiring a full manual
		// regenerate (which was needlessly burning tokens on Plan/Design
		// too, neither of which caused or can fix this).
		if byFile := findMissingComponentImportsByFile(codeOutput.Files); len(byFile) > 0 {
			// One QAIssue per (importing file, missing component) pair,
			// with File set to the ACTUAL importing file — code.tmpl's
			// retry-pass rule has each per-target call check whether QA
			// feedback mentions its own file before acting on it, so an
			// issue with no File attribution would be silently ignored by
			// every target, including the one that needs to fix it.
			var issues []QAIssue
			var files []string
			for path := range byFile {
				files = append(files, path)
			}
			sort.Strings(files)
			missingNamed := map[string]bool{}
			for _, path := range files {
				for _, name := range byFile[path] {
					issues = append(issues, QAIssue{
						File:     path,
						Severity: "blocking",
						Message: fmt.Sprintf(
							"This file imports '%s' from \"@/components/%s\", but that component was never generated — src/components/%s.tsx does not exist. Either generate this component properly (matching the Plan spec's component list), or if it was invented in error, rewrite THIS file to not reference it (inline the needed markup directly instead).",
							name, name, name,
						),
					})
					missingNamed[name] = true
				}
			}
			// The issues above are addressed to the file(s) that IMPORT the
			// missing component — but per code.tmpl's scoping rules, only
			// the component's OWN generation call (TargetComponent == name)
			// is allowed to emit src/components/<name>.tsx, and that call
			// only acts on feedback that names ITS OWN file. Without an
			// issue whose File is the component's own path, nobody is ever
			// actually told to create it: the importer's call sees the
			// feedback but is out of scope to emit a component file, and
			// the component's own call never sees feedback naming itself,
			// so it just returns its (nonexistent) file "unchanged" — a
			// real production case (Header/Footer/ContactForm all silently
			// missing from a build despite self-heal supposedly running).
			plannedComponents := map[string]bool{}
			for _, c := range planOutput.Components {
				plannedComponents[c.Name] = true
			}
			var missingNames []string
			for name := range missingNamed {
				// Only names that are actually in the Plan spec's component
				// list have a TargetComponent generation call to receive
				// this issue at all — a fully invented name (never planned)
				// has no such call, so the only fix possible is the
				// importer removing the reference, which the issue above
				// already asks for.
				if plannedComponents[name] {
					missingNames = append(missingNames, name)
				}
			}
			sort.Strings(missingNames)
			for _, name := range missingNames {
				issues = append(issues, QAIssue{
					File:     "src/components/" + name + ".tsx",
					Severity: "blocking",
					Message: fmt.Sprintf(
						"This component is imported elsewhere in the app but was never generated. Generate src/components/%s.tsx now, exporting a component named %s with a generic/reusable props interface.",
						name, name,
					),
				})
			}
			qaOutput = QAOutput{Passed: false, Issues: issues}
		} else {
			qaErr := workflow.ExecuteActivity(ctx, qaActivityName, QAInput{
				RunID:   in.RunID,
				Attempt: attempt,
				Files:   codeOutput.Files,
				Spec:    planOutput,
				AppKind: in.AppKind,
			}).Get(ctx, &qaOutput)
			if qaErr != nil {
				result = GenerateAppResult{Status: "failed", Error: qaErr.Error()}
				return result, qaErr
			}
		}

		if qaOutput.Passed {
			return finishWithBundle("succeeded", "")
		}

		if attempt == maxQARetries {
			return finishWithBundle("needs_review", "QA failed after max retries")
		}

		// Re-run Code with QA feedback
		updatedCodeOutput, retryErr := generateCode(&designOutput.Tokens, codeOutput.Files, qaOutput.Issues)
		if retryErr != nil {
			result = GenerateAppResult{Status: "failed", Error: retryErr.Error()}
			return result, retryErr
		}
		codeOutput = updatedCodeOutput
	}

	return finishWithBundle("needs_review", "QA failed after max retries")
}

// RedeployWorkflow is what the Deploy button actually triggers — never a
// full regenerate. When NeedsBuild is false (the target run already has a
// built bundle from generation time), this is just a fast promote. When
// true (a previous build failed, or this run was never built), it first
// fetches the run's durably-saved raw source and retries the build, using
// the SAME generated code rather than asking the LLM to produce it again.
func RedeployWorkflow(ctx workflow.Context, in RedeployInput) (result RedeployResult, err error) {
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 15 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 2,
			InitialInterval: 5 * time.Second,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Whatever the final outcome, the deployments row created up front by
	// AppService.Deploy (status 'deploying') must never be left stuck —
	// mark it failed on any error so the UI doesn't show "deploying"
	// forever the way the original, unimplemented deploy stub did.
	defer func() {
		if err != nil {
			failCtx, cancel := workflow.NewDisconnectedContext(ctx)
			defer cancel()
			failCtx = workflow.WithActivityOptions(failCtx, ao)
			_ = workflow.ExecuteActivity(failCtx, markDeploymentFailedActivityName, MarkDeploymentFailedInput{
				DeploymentID: in.DeploymentID,
				Error:        err.Error(),
			}).Get(failCtx, nil)
			result = RedeployResult{Status: "failed", Error: err.Error()}
		}
	}()

	if in.NeedsBuild {
		var srcOut FetchRunSourceOutput
		if err = workflow.ExecuteActivity(ctx, fetchRunSourceActivityName, FetchRunSourceInput{RunID: in.RunID}).Get(ctx, &srcOut); err != nil {
			return result, err
		}

		var pubOut PublishBundleOutput
		if err = workflow.ExecuteActivity(ctx, publishActivityName, PublishBundleInput{RunID: in.RunID, Files: srcOut.Files}).Get(ctx, &pubOut); err != nil {
			return result, err
		}

		if err = workflow.ExecuteActivity(ctx, updateRunBundlePathActivityName, UpdateRunBundlePathInput{
			RunID:      in.RunID,
			BundlePath: pubOut.BundlePath,
		}).Get(ctx, nil); err != nil {
			return result, err
		}
	}

	if err = workflow.ExecuteActivity(ctx, promoteDeploymentActivityName, PromoteDeploymentInput{
		AppID:        in.AppID,
		RunID:        in.RunID,
		DeploymentID: in.DeploymentID,
	}).Get(ctx, nil); err != nil {
		return result, err
	}

	result = RedeployResult{Status: "live"}
	return result, nil
}
