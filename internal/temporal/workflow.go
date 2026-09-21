package temporal

import (
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

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
	planActivityName    = "PlanActivity"
	designActivityName  = "DesignActivity"
	codeActivityName    = "CodeActivity"
	qaActivityName      = "QAActivity"
	persistActivityName = "PersistRunResultActivity"
	publishActivityName = "PublishBundleActivity"
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
	// other in-flight/queued targets are never touched or re-run. If a
	// non-essential target (a page or component) still fails after its
	// retries, it's just skipped: the rest of the app still gets returned
	// and generation continues, rather than discarding every other
	// already-succeeded target and failing the whole run over one
	// persistent failure. The shared/root target is the one exception — an
	// app with no layout.tsx/package.json can't run at all, so its failure
	// after retries is still treated as fatal for this call.
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
						if target.page == nil && target.component == nil {
							// Shared/root files are load-bearing for the
							// whole app — can't gracefully degrade without
							// them, so this one failure IS fatal for the
							// call. Other already in-flight targets are
							// still allowed to finish/drain below; we just
							// stop queuing new work.
							if firstErr == nil {
								firstErr = err
							}
							return
						}
						// Non-essential target exhausted its retries —
						// skip it, keep the rest of the run going.
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
		if len(codeOutput.Files) > 0 {
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
		result = GenerateAppResult{Status: status, Error: errMsg, BundlePath: bundlePath}
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
