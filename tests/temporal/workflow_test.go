package temporal_test

import (
	"context"
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
