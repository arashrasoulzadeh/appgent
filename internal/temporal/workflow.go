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
}

type GenerateAppResult struct {
	Status string
	Error  string
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
	// shared/layout component (Header, Footer, etc. — anything in the Plan
	// spec's component list with Type == "layout"), plus one for the
	// shared/root files (layout.tsx, config, globals.css, manifest/sw for
	// pwa), run as parallel Temporal activities capped at maxParallelCode
	// concurrent at a time, then merges every call's Files into one map.
	// This replaces a single call that generated the whole app's file tree
	// in one LLM response — smaller, focused calls are both faster (real
	// concurrency) and safer (a single call producing truncated/invalid
	// output only loses that one piece, not the entire run). Non-layout
	// components (ui/form/etc.) are deliberately NOT split out here — they
	// risk being generated inconsistently by multiple pages that each
	// reference them, so those stay page-local, generated inline with
	// whichever page(s) use them.
	const maxParallelCode = 5
	type codeTarget struct {
		page      *PageSpec
		component *ComponentSpec
	}
	generateCode := func(tokens *DesignTokens, priorFiles map[string]string, qaFeedback []QAIssue) (CodeOutput, error) {
		codeAttempt++
		attempt := codeAttempt

		targets := make([]codeTarget, 0, len(planOutput.Pages)+len(planOutput.Components)+1)
		targets = append(targets, codeTarget{}) // shared/root files
		for i := range planOutput.Components {
			c := planOutput.Components[i]
			if c.Type == "layout" {
				targets = append(targets, codeTarget{component: &c})
			}
		}
		for i := range planOutput.Pages {
			page := planOutput.Pages[i]
			targets = append(targets, codeTarget{page: &page})
		}

		merged := make(map[string]string)
		for start := 0; start < len(targets); start += maxParallelCode {
			end := start + maxParallelCode
			if end > len(targets) {
				end = len(targets)
			}
			batch := targets[start:end]

			futures := make([]workflow.Future, len(batch))
			for i, target := range batch {
				futures[i] = workflow.ExecuteActivity(codeCtx, codeActivityName, CodeInput{
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
			}
			for _, f := range futures {
				var out CodeOutput
				if err := f.Get(ctx, &out); err != nil {
					return CodeOutput{}, err
				}
				for path, content := range out.Files {
					merged[path] = content
				}
			}
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
			result = GenerateAppResult{Status: "succeeded", Error: ""}
			return result, nil
		}

		if attempt == maxQARetries {
			result = GenerateAppResult{Status: "needs_review", Error: "QA failed after max retries"}
			return result, nil
		}

		// Re-run Code with QA feedback
		updatedCodeOutput, retryErr := generateCode(&designOutput.Tokens, codeOutput.Files, qaOutput.Issues)
		if retryErr != nil {
			result = GenerateAppResult{Status: "failed", Error: retryErr.Error()}
			return result, retryErr
		}
		codeOutput = updatedCodeOutput
	}

	result = GenerateAppResult{Status: "needs_review", Error: "QA failed after max retries"}
	return result, nil
}
