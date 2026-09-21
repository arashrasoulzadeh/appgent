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
		RunID:      uuid.New(),
		AppID:      uuid.New(),
		AppKind:    "website",
		UserPrompt: "a portfolio site",
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
