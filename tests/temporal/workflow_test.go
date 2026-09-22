package temporal_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	apptemporal "github.com/arashrasoulzadeh/appgent/internal/temporal"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
)

func newInput() apptemporal.GenerateAppInput {
	return apptemporal.GenerateAppInput{
		RunID:        uuid.New(),
		AppID:        uuid.New(),
		AppKind:      "website",
		UserPrompt:   "a portfolio site",
		ParallelCode: true,
	}
}

// newTestEnv registers activity signatures under the exact names
// GenerateAppWorkflow references by string (see internal/temporal/workflow.go)
// so env.OnActivity("Name", ...) below has something to attach mocked
// behavior to — the test environment requires a name to already be known
// before it can be overridden.
func newTestEnv(suite *testsuite.WorkflowTestSuite) *testsuite.TestWorkflowEnvironment {
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterActivityWithOptions(
		func(context.Context, apptemporal.PlanInput) (apptemporal.PlanOutput, error) {
			return apptemporal.PlanOutput{}, nil
		},
		activity.RegisterOptions{Name: "PlanActivity"},
	)
	env.RegisterActivityWithOptions(
		func(context.Context, apptemporal.DesignInput) (apptemporal.DesignOutput, error) {
			return apptemporal.DesignOutput{}, nil
		},
		activity.RegisterOptions{Name: "DesignActivity"},
	)
	env.RegisterActivityWithOptions(
		func(context.Context, apptemporal.CodeInput) (apptemporal.CodeOutput, error) {
			return apptemporal.CodeOutput{}, nil
		},
		activity.RegisterOptions{Name: "CodeActivity"},
	)
	env.RegisterActivityWithOptions(
		func(context.Context, apptemporal.QAInput) (apptemporal.QAOutput, error) {
			return apptemporal.QAOutput{}, nil
		},
		activity.RegisterOptions{Name: "QAActivity"},
	)
	env.RegisterActivityWithOptions(
		func(context.Context, uuid.UUID, uuid.UUID, apptemporal.GenerateAppResult) error { return nil },
		activity.RegisterOptions{Name: "PersistRunResultActivity"},
	)
	env.RegisterActivityWithOptions(
		func(context.Context, apptemporal.PublishBundleInput) (apptemporal.PublishBundleOutput, error) {
			return apptemporal.PublishBundleOutput{}, nil
		},
		activity.RegisterOptions{Name: "PublishBundleActivity"},
	)
	env.RegisterActivityWithOptions(
		func(context.Context, apptemporal.PublishSourceInput) (apptemporal.PublishSourceOutput, error) {
			return apptemporal.PublishSourceOutput{}, nil
		},
		activity.RegisterOptions{Name: "PublishSourceActivity"},
	)
	env.RegisterActivityWithOptions(
		func(context.Context, apptemporal.FetchRunSourceInput) (apptemporal.FetchRunSourceOutput, error) {
			return apptemporal.FetchRunSourceOutput{}, nil
		},
		activity.RegisterOptions{Name: "FetchRunSourceActivity"},
	)
	env.RegisterActivityWithOptions(
		func(context.Context, apptemporal.UpdateRunBundlePathInput) error { return nil },
		activity.RegisterOptions{Name: "UpdateRunBundlePathActivity"},
	)
	env.RegisterActivityWithOptions(
		func(context.Context, apptemporal.PromoteDeploymentInput) (apptemporal.PromoteDeploymentOutput, error) {
			return apptemporal.PromoteDeploymentOutput{}, nil
		},
		activity.RegisterOptions{Name: "PromoteDeploymentActivity"},
	)
	env.RegisterActivityWithOptions(
		func(context.Context, apptemporal.MarkDeploymentFailedInput) error { return nil },
		activity.RegisterOptions{Name: "MarkDeploymentFailedActivity"},
	)
	return env
}

// TestGenerateAppWorkflow_HappyPath verifies Plan -> Design||Code -> QA(pass)
// succeeds, and that the persist activity is invoked with the successful
// result (regression test for the exit-path persist wiring — the workflow
// previously never called PersistRunResultActivity at all).
func TestGenerateAppWorkflow_HappyPath(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := newTestEnv(&suite)

	env.OnActivity("PlanActivity", mock.Anything, mock.Anything).
		Return(apptemporal.PlanOutput{StyleDirection: "clean"}, nil)
	env.OnActivity("DesignActivity", mock.Anything, mock.Anything).
		Return(apptemporal.DesignOutput{}, nil)
	env.OnActivity("CodeActivity", mock.Anything, mock.Anything).
		Return(apptemporal.CodeOutput{Files: map[string]string{"a.tsx": "x"}}, nil)
	env.OnActivity("QAActivity", mock.Anything, mock.Anything).
		Return(apptemporal.QAOutput{Passed: true}, nil)

	var persistedStatus string
	env.OnActivity("PersistRunResultActivity", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			result := args.Get(3).(apptemporal.GenerateAppResult)
			persistedStatus = result.Status
		}).
		Return(nil)

	env.ExecuteWorkflow(apptemporal.GenerateAppWorkflow, newInput())

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result apptemporal.GenerateAppResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "succeeded", result.Status)
	require.Equal(t, "succeeded", persistedStatus, "PersistRunResultActivity must run with the final result")

	env.AssertExpectations(t)
}

// TestGenerateAppWorkflow_QARetryThenSucceeds verifies a failing QA pass
// triggers a Code re-run with the QA feedback, and a subsequent pass
// succeeds the workflow.
func TestGenerateAppWorkflow_QARetryThenSucceeds(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := newTestEnv(&suite)

	input := newInput()

	env.OnActivity("PlanActivity", mock.Anything, mock.Anything).
		Return(apptemporal.PlanOutput{}, nil)
	// A non-zero Colors value is required to exercise the design-token
	// refresh branch in GenerateAppWorkflow (a zero-value DesignOutput, as
	// an unconfigured mock would otherwise return, is treated as "design
	// didn't produce real tokens" and the refresh Code call is skipped).
	env.OnActivity("DesignActivity", mock.Anything, mock.Anything).
		Return(apptemporal.DesignOutput{
			Tokens: apptemporal.DesignTokens{
				Colors: apptemporal.ColorPalette{Primary: apptemporal.ColorVariant{Main: "#123456"}},
			},
		}, nil)

	var codeAttempts []int
	var codeRunIDs []uuid.UUID
	env.OnActivity("CodeActivity", mock.Anything, mock.Anything).
		Return(func(_ context.Context, in apptemporal.CodeInput) (apptemporal.CodeOutput, error) {
			codeAttempts = append(codeAttempts, in.Attempt)
			codeRunIDs = append(codeRunIDs, in.RunID)
			return apptemporal.CodeOutput{Files: map[string]string{"a.tsx": "x"}}, nil
		})

	var qaAttempts []int
	env.OnActivity("QAActivity", mock.Anything, mock.Anything).
		Return(func(_ context.Context, in apptemporal.QAInput) (apptemporal.QAOutput, error) {
			qaAttempts = append(qaAttempts, in.Attempt)
			if len(qaAttempts) == 1 {
				return apptemporal.QAOutput{Passed: false, Issues: []apptemporal.QAIssue{{Message: "broken link"}}}, nil
			}
			return apptemporal.QAOutput{Passed: true}, nil
		})

	env.OnActivity("PersistRunResultActivity", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	env.ExecuteWorkflow(apptemporal.GenerateAppWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result apptemporal.GenerateAppResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "succeeded", result.Status)
	require.Equal(t, []int{1, 2}, qaAttempts, "QA attempt numbers should be 1, then 2 on retry")
	// Code runs: once initially (parallel with Design), once more after the
	// design-tokens re-run, once more for the QA retry — attempts 1, 2, 3,
	// each distinct so the run-detail timeline can show every generation
	// pass separately instead of collapsing them into one step.
	require.Equal(t, []int{1, 2, 3}, codeAttempts, "each Code call should get a distinct, increasing attempt number")
	for _, id := range codeRunIDs {
		require.Equal(t, input.RunID, id, "every activity call must carry the run's RunID for step tracking")
	}

	env.AssertExpectations(t)
}

// TestGenerateAppWorkflow_QAFailsAllRetries verifies the workflow ends in
// needs_review (not an error) once QA fails every retry, and that this
// terminal state is what gets persisted.
func TestGenerateAppWorkflow_QAFailsAllRetries(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := newTestEnv(&suite)

	env.OnActivity("PlanActivity", mock.Anything, mock.Anything).
		Return(apptemporal.PlanOutput{}, nil)
	env.OnActivity("DesignActivity", mock.Anything, mock.Anything).
		Return(apptemporal.DesignOutput{}, nil)
	env.OnActivity("CodeActivity", mock.Anything, mock.Anything).
		Return(apptemporal.CodeOutput{Files: map[string]string{"a.tsx": "x"}}, nil)
	env.OnActivity("QAActivity", mock.Anything, mock.Anything).
		Return(apptemporal.QAOutput{Passed: false, Issues: []apptemporal.QAIssue{{Message: "still broken"}}}, nil)

	var persistedStatus string
	env.OnActivity("PersistRunResultActivity", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			result := args.Get(3).(apptemporal.GenerateAppResult)
			persistedStatus = result.Status
		}).
		Return(nil)

	env.ExecuteWorkflow(apptemporal.GenerateAppWorkflow, newInput())

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result apptemporal.GenerateAppResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "needs_review", result.Status)
	require.Equal(t, "needs_review", persistedStatus)

	env.AssertExpectations(t)
}

// TestGenerateAppWorkflow_PlanFails_StillPersists is the key regression
// test: before this fix, a failed Plan activity returned an error directly
// without ever invoking PersistRunResultActivity, so generation_runs.status
// stayed "queued" forever even though the run had actually failed.
func TestGenerateAppWorkflow_PlanFails_StillPersists(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := newTestEnv(&suite)

	env.OnActivity("PlanActivity", mock.Anything, mock.Anything).
		Return(apptemporal.PlanOutput{}, assertError("plan exploded"))

	var persistedStatus, persistedError string
	env.OnActivity("PersistRunResultActivity", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			result := args.Get(3).(apptemporal.GenerateAppResult)
			persistedStatus = result.Status
			persistedError = result.Error
		}).
		Return(nil)

	env.ExecuteWorkflow(apptemporal.GenerateAppWorkflow, newInput())

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
	require.Equal(t, "failed", persistedStatus, "PersistRunResultActivity must still run when Plan fails")
	require.NotEmpty(t, persistedError)

	env.AssertExpectations(t)
}

type assertError string

func (e assertError) Error() string { return string(e) }

// TestGenerateAppWorkflow_CodeFansOutByPage verifies code generation is
// split into one parallel call per page plus one for the shared/root files
// (7 pages + 1 shared = 8 targets, batched in groups of 5 — this exercises
// both the fan-out and the batching-loop boundary), and that every call's
// Files get merged into the final CodeOutput without dropping any.
func TestGenerateAppWorkflow_CodeFansOutByPage(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := newTestEnv(&suite)

	pages := make([]apptemporal.PageSpec, 7)
	for i := range pages {
		pages[i] = apptemporal.PageSpec{Name: fmt.Sprintf("Page%d", i), Path: fmt.Sprintf("/page%d", i)}
	}

	env.OnActivity("PlanActivity", mock.Anything, mock.Anything).
		Return(apptemporal.PlanOutput{Pages: pages}, nil)
	env.OnActivity("DesignActivity", mock.Anything, mock.Anything).
		Return(apptemporal.DesignOutput{}, nil)

	var mu sync.Mutex
	seenTargets := map[string]bool{}
	env.OnActivity("CodeActivity", mock.Anything, mock.Anything).
		Return(func(_ context.Context, in apptemporal.CodeInput) (apptemporal.CodeOutput, error) {
			key := "shared"
			if in.TargetPage != nil {
				key = in.TargetPage.Name
			}
			mu.Lock()
			seenTargets[key] = true
			mu.Unlock()
			return apptemporal.CodeOutput{Files: map[string]string{key + ".tsx": "content-for-" + key}}, nil
		})
	env.OnActivity("QAActivity", mock.Anything, mock.Anything).
		Return(apptemporal.QAOutput{Passed: true}, nil)
	env.OnActivity("PersistRunResultActivity", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	env.ExecuteWorkflow(apptemporal.GenerateAppWorkflow, newInput())

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result apptemporal.GenerateAppResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "succeeded", result.Status)

	// 1 shared target + 7 pages, none dropped across the batch-of-5 loop.
	require.Len(t, seenTargets, 8)
	require.True(t, seenTargets["shared"])
	for _, p := range pages {
		require.True(t, seenTargets[p.Name], "page %s should have gotten its own CodeActivity call", p.Name)
	}

	env.AssertExpectations(t)
}

// TestGenerateAppWorkflow_CodeFansOutByComponentAndPage verifies that EVERY
// component in the Plan spec's list — regardless of type (layout, ui, form,
// ...) — gets its own parallel CodeActivity call, same as pages, so no
// component is ever regenerated redundantly by multiple pages.
func TestGenerateAppWorkflow_CodeFansOutByComponentAndPage(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := newTestEnv(&suite)

	pages := []apptemporal.PageSpec{{Name: "Home", Path: "/"}, {Name: "About", Path: "/about"}}
	components := []apptemporal.ComponentSpec{
		{Name: "Header", Type: "layout"},
		{Name: "Footer", Type: "layout"},
		{Name: "ProjectCard", Type: "ui"}, // non-layout — still gets its own call
	}

	env.OnActivity("PlanActivity", mock.Anything, mock.Anything).
		Return(apptemporal.PlanOutput{Pages: pages, Components: components}, nil)
	env.OnActivity("DesignActivity", mock.Anything, mock.Anything).
		Return(apptemporal.DesignOutput{}, nil)

	var mu sync.Mutex
	seenTargets := map[string]bool{}
	env.OnActivity("CodeActivity", mock.Anything, mock.Anything).
		Return(func(_ context.Context, in apptemporal.CodeInput) (apptemporal.CodeOutput, error) {
			key := "shared"
			switch {
			case in.TargetPage != nil:
				key = "page:" + in.TargetPage.Name
			case in.TargetComponent != nil:
				key = "component:" + in.TargetComponent.Name
			}
			mu.Lock()
			seenTargets[key] = true
			mu.Unlock()
			return apptemporal.CodeOutput{Files: map[string]string{key + ".tsx": "x"}}, nil
		})
	env.OnActivity("QAActivity", mock.Anything, mock.Anything).
		Return(apptemporal.QAOutput{Passed: true}, nil)
	env.OnActivity("PersistRunResultActivity", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	env.ExecuteWorkflow(apptemporal.GenerateAppWorkflow, newInput())

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result apptemporal.GenerateAppResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "succeeded", result.Status)

	// shared + 3 components (all types) + 2 pages = 6 calls.
	require.Len(t, seenTargets, 6)
	require.True(t, seenTargets["shared"])
	require.True(t, seenTargets["component:Header"])
	require.True(t, seenTargets["component:Footer"])
	require.True(t, seenTargets["component:ProjectCard"], "non-layout components must also get their own call")
	require.True(t, seenTargets["page:Home"])
	require.True(t, seenTargets["page:About"])

	env.AssertExpectations(t)
}

// TestGenerateAppWorkflow_CodeRetriesOnlyFailedTarget verifies that when one
// concurrent CodeActivity call fails (after exhausting its own Temporal
// RetryPolicy), only that ONE target is retried — other already-succeeded
// or still in-flight targets are neither re-run nor discarded, and a
// non-essential target failing after its extra retry doesn't fail the run.
func TestGenerateAppWorkflow_CodeRetriesOnlyFailedTarget(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := newTestEnv(&suite)

	pages := []apptemporal.PageSpec{{Name: "Home", Path: "/"}, {Name: "About", Path: "/about"}}

	env.OnActivity("PlanActivity", mock.Anything, mock.Anything).
		Return(apptemporal.PlanOutput{Pages: pages}, nil)
	env.OnActivity("DesignActivity", mock.Anything, mock.Anything).
		Return(apptemporal.DesignOutput{}, nil)

	var mu sync.Mutex
	callCounts := map[string]int{}
	env.OnActivity("CodeActivity", mock.Anything, mock.Anything).
		Return(func(_ context.Context, in apptemporal.CodeInput) (apptemporal.CodeOutput, error) {
			key := "shared"
			switch {
			case in.TargetPage != nil:
				key = "page:" + in.TargetPage.Name
			case in.TargetComponent != nil:
				key = "component:" + in.TargetComponent.Name
			}
			mu.Lock()
			callCounts[key]++
			n := callCounts[key]
			mu.Unlock()
			if key == "page:About" && n == 1 {
				return apptemporal.CodeOutput{}, fmt.Errorf("transient failure")
			}
			return apptemporal.CodeOutput{Files: map[string]string{key + ".tsx": "x"}}, nil
		})
	env.OnActivity("QAActivity", mock.Anything, mock.Anything).
		Return(apptemporal.QAOutput{Passed: true}, nil)
	env.OnActivity("PersistRunResultActivity", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	env.ExecuteWorkflow(apptemporal.GenerateAppWorkflow, newInput())

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result apptemporal.GenerateAppResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "succeeded", result.Status)

	mu.Lock()
	defer mu.Unlock()
	// The failed target ("page:About") was retried exactly once more and
	// succeeded on its second call. Every other target was called exactly
	// once — a single failure never caused any other target to re-run.
	require.Equal(t, 2, callCounts["page:About"])
	require.Equal(t, 1, callCounts["page:Home"])
	require.Equal(t, 1, callCounts["shared"])

	env.AssertExpectations(t)
}

// TestGenerateAppWorkflow_SequentialCodeWhenParallelDisabled verifies that
// setting ParallelCode: false makes code generation run strictly one target
// at a time — never more than one CodeActivity call in flight — instead of
// the default rolling window of up to 5 concurrent calls.
func TestGenerateAppWorkflow_SequentialCodeWhenParallelDisabled(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := newTestEnv(&suite)

	pages := []apptemporal.PageSpec{
		{Name: "Home", Path: "/"},
		{Name: "About", Path: "/about"},
		{Name: "Contact", Path: "/contact"},
	}

	env.OnActivity("PlanActivity", mock.Anything, mock.Anything).
		Return(apptemporal.PlanOutput{Pages: pages}, nil)
	env.OnActivity("DesignActivity", mock.Anything, mock.Anything).
		Return(apptemporal.DesignOutput{}, nil)

	var mu sync.Mutex
	inFlight := 0
	maxInFlight := 0
	calls := 0
	env.OnActivity("CodeActivity", mock.Anything, mock.Anything).
		Return(func(_ context.Context, in apptemporal.CodeInput) (apptemporal.CodeOutput, error) {
			mu.Lock()
			inFlight++
			calls++
			if inFlight > maxInFlight {
				maxInFlight = inFlight
			}
			key := "shared"
			if in.TargetPage != nil {
				key = in.TargetPage.Name
			}
			mu.Unlock()

			mu.Lock()
			inFlight--
			mu.Unlock()
			return apptemporal.CodeOutput{Files: map[string]string{key + ".tsx": "x"}}, nil
		})
	env.OnActivity("QAActivity", mock.Anything, mock.Anything).
		Return(apptemporal.QAOutput{Passed: true}, nil)
	env.OnActivity("PersistRunResultActivity", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	in := newInput()
	in.ParallelCode = false
	env.ExecuteWorkflow(apptemporal.GenerateAppWorkflow, in)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result apptemporal.GenerateAppResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "succeeded", result.Status)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 4, calls) // 1 shared + 3 pages
	require.Equal(t, 1, maxInFlight, "ParallelCode: false must never have more than 1 CodeActivity call in flight")

	env.AssertExpectations(t)
}

// TestGenerateAppWorkflow_PublishesBundleOnSuccess is a regression test for
// the bug where a succeeded/needs_review run never actually published its
// generated files anywhere — PersistRunResult only ever wrote status/error,
// so bundle_path stayed NULL forever and preview/deploy were permanently
// broken. Asserts PublishBundleActivity is called with the final merged
// files and its BundlePath ends up on the workflow result.
func TestGenerateAppWorkflow_PublishesBundleOnSuccess(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := newTestEnv(&suite)

	env.OnActivity("PlanActivity", mock.Anything, mock.Anything).
		Return(apptemporal.PlanOutput{Pages: []apptemporal.PageSpec{{Name: "Home", Path: "/"}}}, nil)
	env.OnActivity("DesignActivity", mock.Anything, mock.Anything).
		Return(apptemporal.DesignOutput{}, nil)
	env.OnActivity("CodeActivity", mock.Anything, mock.Anything).
		Return(func(_ context.Context, in apptemporal.CodeInput) (apptemporal.CodeOutput, error) {
			key := "shared.tsx"
			if in.TargetPage != nil {
				key = in.TargetPage.Name + ".tsx"
			}
			return apptemporal.CodeOutput{Files: map[string]string{key: "content"}}, nil
		})
	env.OnActivity("QAActivity", mock.Anything, mock.Anything).
		Return(apptemporal.QAOutput{Passed: true}, nil)
	env.OnActivity("PersistRunResultActivity", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	var publishedFiles map[string]string
	env.OnActivity("PublishBundleActivity", mock.Anything, mock.Anything).
		Return(func(_ context.Context, in apptemporal.PublishBundleInput) (apptemporal.PublishBundleOutput, error) {
			publishedFiles = in.Files
			return apptemporal.PublishBundleOutput{BundlePath: "runs/" + in.RunID.String() + "/"}, nil
		})

	in := newInput()
	env.ExecuteWorkflow(apptemporal.GenerateAppWorkflow, in)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result apptemporal.GenerateAppResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "succeeded", result.Status)
	require.Equal(t, "runs/"+in.RunID.String()+"/", result.BundlePath)

	require.Equal(t, map[string]string{"shared.tsx": "content", "Home.tsx": "content"}, publishedFiles)

	env.AssertExpectations(t)
}

// TestGenerateAppWorkflow_PublishFailureDoesNotFailRun verifies that a
// PublishBundleActivity failure is non-fatal: the run still ends up
// "succeeded" (generation itself worked), just with an empty BundlePath —
// deliberately not turning a working generation into a failed run just
// because publishing the bundle for preview/deploy didn't work.
func TestGenerateAppWorkflow_PublishFailureDoesNotFailRun(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := newTestEnv(&suite)

	env.OnActivity("PlanActivity", mock.Anything, mock.Anything).
		Return(apptemporal.PlanOutput{Pages: []apptemporal.PageSpec{{Name: "Home", Path: "/"}}}, nil)
	env.OnActivity("DesignActivity", mock.Anything, mock.Anything).
		Return(apptemporal.DesignOutput{}, nil)
	env.OnActivity("CodeActivity", mock.Anything, mock.Anything).
		Return(apptemporal.CodeOutput{Files: map[string]string{"index.tsx": "x"}}, nil)
	env.OnActivity("QAActivity", mock.Anything, mock.Anything).
		Return(apptemporal.QAOutput{Passed: true}, nil)
	env.OnActivity("PersistRunResultActivity", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil)
	env.OnActivity("PublishBundleActivity", mock.Anything, mock.Anything).
		Return(apptemporal.PublishBundleOutput{}, fmt.Errorf("object storage unreachable"))

	env.ExecuteWorkflow(apptemporal.GenerateAppWorkflow, newInput())

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result apptemporal.GenerateAppResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "succeeded", result.Status)
	require.Empty(t, result.BundlePath)

	env.AssertExpectations(t)
}

// TestGenerateAppWorkflow_ComponentFailureIsFatal is a regression test for
// a real production bug: a component (e.g. Header, ContactForm) failing
// after its retries was treated like a skippable page and silently
// dropped, but other already-generated files (layout.tsx, pages) `import`
// components by name — a missing component guarantees a build failure
// ("Module not found"), it doesn't degrade gracefully like a missing page
// does. Only page-target failures should be non-fatal.
func TestGenerateAppWorkflow_ComponentFailureIsFatal(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := newTestEnv(&suite)

	env.OnActivity("PlanActivity", mock.Anything, mock.Anything).
		Return(apptemporal.PlanOutput{
			Pages:      []apptemporal.PageSpec{{Name: "Home", Path: "/"}},
			Components: []apptemporal.ComponentSpec{{Name: "Header", Type: "layout"}},
		}, nil)
	env.OnActivity("DesignActivity", mock.Anything, mock.Anything).
		Return(apptemporal.DesignOutput{}, nil)
	env.OnActivity("CodeActivity", mock.Anything, mock.Anything).
		Return(func(_ context.Context, in apptemporal.CodeInput) (apptemporal.CodeOutput, error) {
			if in.TargetComponent != nil && in.TargetComponent.Name == "Header" {
				return apptemporal.CodeOutput{}, fmt.Errorf("model returned invalid output")
			}
			key := "shared"
			if in.TargetPage != nil {
				key = in.TargetPage.Name
			}
			return apptemporal.CodeOutput{Files: map[string]string{key + ".tsx": "x"}}, nil
		})
	// QAActivity is deliberately not mocked here — the component failure
	// must be fatal before generation ever reaches the QA loop.

	var persistedStatus string
	env.OnActivity("PersistRunResultActivity", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			persistedStatus = args.Get(3).(apptemporal.GenerateAppResult).Status
		}).
		Return(nil)

	env.ExecuteWorkflow(apptemporal.GenerateAppWorkflow, newInput())

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
	require.Equal(t, "failed", persistedStatus, "a persistently failing component must fail the run, not silently ship without it")

	env.AssertExpectations(t)
}

func newRedeployInput(needsBuild bool) apptemporal.RedeployInput {
	return apptemporal.RedeployInput{
		AppID:        uuid.New(),
		RunID:        uuid.New(),
		DeploymentID: uuid.New(),
		NeedsBuild:   needsBuild,
	}
}

// TestRedeployWorkflow_FastPath_NoBuildNeeded verifies that when the
// target run already has a built bundle, RedeployWorkflow skips straight
// to promoting it — no source fetch, no rebuild. This is the common case:
// clicking Deploy again for a run that already built successfully.
func TestRedeployWorkflow_FastPath_NoBuildNeeded(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := newTestEnv(&suite)

	env.OnActivity("PromoteDeploymentActivity", mock.Anything, mock.Anything).
		Return(apptemporal.PromoteDeploymentOutput{URL: "/api/v1/apps/x/live/index.html"}, nil)

	in := newRedeployInput(false)
	env.ExecuteWorkflow(apptemporal.RedeployWorkflow, in)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result apptemporal.RedeployResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "live", result.Status)

	// FetchRunSourceActivity/PublishBundleActivity/UpdateRunBundlePathActivity
	// must never be called — nothing to build, nothing to fetch.
	env.AssertNotCalled(t, "FetchRunSourceActivity", mock.Anything, mock.Anything)
	env.AssertNotCalled(t, "PublishBundleActivity", mock.Anything, mock.Anything)
	env.AssertExpectations(t)
}

// TestRedeployWorkflow_NeedsBuild_RebuildsFromSavedSource verifies the
// actual feature the user asked for: Deploy on a run whose build
// previously failed (only source_path was saved) fetches that saved
// source and rebuilds it — using the SAME generated code, not asking the
// LLM to regenerate anything — then promotes the result.
func TestRedeployWorkflow_NeedsBuild_RebuildsFromSavedSource(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := newTestEnv(&suite)

	savedSource := map[string]string{"src/app/page.tsx": "export default function Page() {}"}
	env.OnActivity("FetchRunSourceActivity", mock.Anything, mock.Anything).
		Return(apptemporal.FetchRunSourceOutput{Files: savedSource}, nil)

	var publishedFiles map[string]string
	env.OnActivity("PublishBundleActivity", mock.Anything, mock.Anything).
		Return(func(_ context.Context, in apptemporal.PublishBundleInput) (apptemporal.PublishBundleOutput, error) {
			publishedFiles = in.Files
			return apptemporal.PublishBundleOutput{BundlePath: "runs/" + in.RunID.String() + "/"}, nil
		})

	var updatedBundlePath string
	env.OnActivity("UpdateRunBundlePathActivity", mock.Anything, mock.Anything).
		Return(func(_ context.Context, in apptemporal.UpdateRunBundlePathInput) error {
			updatedBundlePath = in.BundlePath
			return nil
		})

	env.OnActivity("PromoteDeploymentActivity", mock.Anything, mock.Anything).
		Return(apptemporal.PromoteDeploymentOutput{URL: "/api/v1/apps/x/live/index.html"}, nil)

	in := newRedeployInput(true)
	env.ExecuteWorkflow(apptemporal.RedeployWorkflow, in)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result apptemporal.RedeployResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "live", result.Status)

	require.Equal(t, savedSource, publishedFiles, "must rebuild using the run's saved source, not regenerate")
	require.Equal(t, "runs/"+in.RunID.String()+"/", updatedBundlePath)

	env.AssertExpectations(t)
}

// TestRedeployWorkflow_BuildFailure_MarksDeploymentFailed verifies that a
// failed rebuild marks the deployment row 'failed' rather than leaving it
// stuck at 'deploying' forever — the exact failure mode of the original,
// unimplemented deploy stub this whole feature replaces.
func TestRedeployWorkflow_BuildFailure_MarksDeploymentFailed(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := newTestEnv(&suite)

	env.OnActivity("FetchRunSourceActivity", mock.Anything, mock.Anything).
		Return(apptemporal.FetchRunSourceOutput{Files: map[string]string{"page.tsx": "x"}}, nil)
	env.OnActivity("PublishBundleActivity", mock.Anything, mock.Anything).
		Return(apptemporal.PublishBundleOutput{}, assertError("build failed: module not found"))

	var failedDeploymentID uuid.UUID
	env.OnActivity("MarkDeploymentFailedActivity", mock.Anything, mock.Anything).
		Return(func(_ context.Context, in apptemporal.MarkDeploymentFailedInput) error {
			failedDeploymentID = in.DeploymentID
			return nil
		})

	in := newRedeployInput(true)
	env.ExecuteWorkflow(apptemporal.RedeployWorkflow, in)

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
	require.Equal(t, in.DeploymentID, failedDeploymentID, "must mark the SAME deployment row that was passed in as failed")

	// A failed rebuild must never reach the promote step.
	env.AssertNotCalled(t, "PromoteDeploymentActivity", mock.Anything, mock.Anything)
	env.AssertExpectations(t)
}

// TestGenerateAppWorkflow_MissingComponentImport_SelfHeals is a regression
// test for a real production build failure: a page imported
// "@/components/GhostThing", a name that was never in the Plan spec's
// component list and so was never generated as its own target — this is
// a guaranteed "Module not found" build failure no matter how many times
// the SAME already-succeeded target gets retried (failure isolation in
// generateCode doesn't apply here; nothing ever failed). The workflow
// must catch this with a zero-token static check (never calling the real
// QAActivity while the bad import is still present) and self-heal via the
// existing retry-with-feedback loop, rather than requiring a full,
// separately-triggered regenerate.
func TestGenerateAppWorkflow_MissingComponentImport_SelfHeals(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := newTestEnv(&suite)

	env.OnActivity("PlanActivity", mock.Anything, mock.Anything).
		Return(apptemporal.PlanOutput{Pages: []apptemporal.PageSpec{{Name: "Home", Path: "/"}}}, nil)
	// No design tokens set (zero-value ColorPalette), so the design-
	// refresh branch in GenerateAppWorkflow is skipped and CodeInput.Attempt
	// maps 1:1 to real generateCode invocations, keeping this test simple.
	env.OnActivity("DesignActivity", mock.Anything, mock.Anything).
		Return(apptemporal.DesignOutput{}, nil)

	var retryFeedback []apptemporal.QAIssue
	env.OnActivity("CodeActivity", mock.Anything, mock.Anything).
		Return(func(_ context.Context, in apptemporal.CodeInput) (apptemporal.CodeOutput, error) {
			key := "shared"
			if in.TargetPage != nil {
				key = in.TargetPage.Name
			}
			content := "export default function X() { return null }"
			if in.TargetPage != nil && in.TargetPage.Name == "Home" && in.Attempt == 1 {
				// Invents a component that was never in the Plan's
				// component list — nothing will ever generate it.
				content = `import GhostThing from "@/components/GhostThing"`
			}
			if in.TargetPage != nil && in.TargetPage.Name == "Home" && in.Attempt == 2 {
				retryFeedback = in.QAFeedback
			}
			return apptemporal.CodeOutput{Files: map[string]string{key + ".tsx": content}}, nil
		})

	var qaCalls int
	env.OnActivity("QAActivity", mock.Anything, mock.Anything).
		Return(func(context.Context, apptemporal.QAInput) (apptemporal.QAOutput, error) {
			qaCalls++
			return apptemporal.QAOutput{Passed: true}, nil
		})

	env.OnActivity("PersistRunResultActivity", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	env.ExecuteWorkflow(apptemporal.GenerateAppWorkflow, newInput())

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result apptemporal.GenerateAppResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "succeeded", result.Status, "must self-heal within the existing retry budget, not require a separate regenerate")

	// The real QA activity must be skipped entirely while the bad import
	// is present (zero LLM tokens spent detecting it) — only called once,
	// on the second attempt, after the retry regenerated Home without the
	// bad import.
	require.Equal(t, 1, qaCalls, "QAActivity must not be called while the structural check would already catch the failure")

	// Regression check: the synthesized QAIssue must set File to the
	// ACTUAL importing file (Home.tsx) — code.tmpl's retry-pass rule has
	// each target check whether QA feedback mentions its own file before
	// acting on it, so an issue with no File attribution would be
	// silently ignored by every target, including Home itself.
	require.Len(t, retryFeedback, 1)
	require.Equal(t, "Home.tsx", retryFeedback[0].File, "QAIssue.File must identify the file with the bad import, or no target will recognize it as theirs to fix")
	require.Contains(t, retryFeedback[0].Message, "GhostThing")

	env.AssertExpectations(t)
}

// TestGenerateAppWorkflow_MissingComponentImport_StubbedAsLastResort covers
// the case the self-heal retry loop alone can't guarantee: the LLM keeps
// regenerating the exact same hallucinated "@/components/GhostThing"
// import on every attempt, exhausting all retries with the bad import
// still present. The workflow must still publish something that BUILDS —
// stubbing the missing component out — rather than reaching the build
// step with a guaranteed "Module not found" failure after retries are
// already spent.
func TestGenerateAppWorkflow_MissingComponentImport_StubbedAsLastResort(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := newTestEnv(&suite)

	env.OnActivity("PlanActivity", mock.Anything, mock.Anything).
		Return(apptemporal.PlanOutput{Pages: []apptemporal.PageSpec{{Name: "Home", Path: "/"}}}, nil)
	env.OnActivity("DesignActivity", mock.Anything, mock.Anything).
		Return(apptemporal.DesignOutput{}, nil)

	// Always references the never-generated component, no matter which
	// attempt this is — simulating an LLM that never actually fixes it.
	env.OnActivity("CodeActivity", mock.Anything, mock.Anything).
		Return(func(_ context.Context, in apptemporal.CodeInput) (apptemporal.CodeOutput, error) {
			key := "shared"
			content := "export default function X() { return null }"
			if in.TargetPage != nil {
				key = in.TargetPage.Name
				content = `import GhostThing from "@/components/GhostThing"`
			}
			return apptemporal.CodeOutput{Files: map[string]string{key + ".tsx": content}}, nil
		})
	// QAActivity is deliberately not mocked — the structural check must
	// catch this on every attempt, never falling through to a real QA
	// call, all the way to retry exhaustion.

	var publishedFiles map[string]string
	env.OnActivity("PublishBundleActivity", mock.Anything, mock.Anything).
		Return(func(_ context.Context, in apptemporal.PublishBundleInput) (apptemporal.PublishBundleOutput, error) {
			publishedFiles = in.Files
			return apptemporal.PublishBundleOutput{BundlePath: "runs/" + in.RunID.String() + "/"}, nil
		})
	env.OnActivity("PersistRunResultActivity", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	env.ExecuteWorkflow(apptemporal.GenerateAppWorkflow, newInput())

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result apptemporal.GenerateAppResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "needs_review", result.Status, "retries genuinely exhausted with the bad import still present")

	require.Contains(t, publishedFiles, "src/components/GhostThing.tsx", "must stub the still-missing component before publishing rather than let the build fail")
	require.Contains(t, publishedFiles["src/components/GhostThing.tsx"], "export default")

	env.AssertExpectations(t)
}
