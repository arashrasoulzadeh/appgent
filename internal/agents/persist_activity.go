package agents

import (
	"context"
	"fmt"

	"github.com/arashrasoulzadeh/appgent/internal/sandbox"
	"github.com/arashrasoulzadeh/appgent/internal/temporal"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PersistActivities holds dependencies (a DB pool, and for deployment
// activities, a Provisioner) for activities that can't be plain functions.
// Registered on the worker via bound methods so GenerateAppWorkflow's
// persist-on-every-exit-path defer and RedeployWorkflow's deployment
// activities have somewhere to actually write to Postgres/object storage.
type PersistActivities struct {
	Pool        *pgxpool.Pool
	Provisioner sandbox.Provisioner
}

// AppStatusFor maps a workflow's terminal run_status to the app_status enum,
// which uses "ready" (not "succeeded") for a completed app.
func AppStatusFor(runStatus string) string {
	switch runStatus {
	case "succeeded":
		return "ready"
	case "needs_review":
		return "needs_review"
	default:
		return "failed"
	}
}

func (a *PersistActivities) PersistRunResult(ctx context.Context, runID, appID uuid.UUID, result temporal.GenerateAppResult) error {
	tx, err := a.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var errText *string
	if result.Error != "" {
		errText = &result.Error
	}

	var bundlePath *string
	if result.BundlePath != "" {
		bundlePath = &result.BundlePath
	}
	var sourcePath *string
	if result.SourcePath != "" {
		sourcePath = &result.SourcePath
	}

	_, err = tx.Exec(ctx, `
		UPDATE generation_runs
		SET status = $1, error = $2, finished_at = now(), bundle_path = $4, source_path = $5
		WHERE id = $3
	`, result.Status, errText, runID, bundlePath, sourcePath)
	if err != nil {
		return fmt.Errorf("update generation_runs: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE apps SET status = $1, updated_at = now() WHERE id = $2
	`, AppStatusFor(result.Status), appID)
	if err != nil {
		return fmt.Errorf("update apps: %w", err)
	}

	return tx.Commit(ctx)
}

// UpdateRunBundlePath records that RedeployWorkflow successfully rebuilt a
// run's bundle (previous build had failed, only source_path was set).
func (a *PersistActivities) UpdateRunBundlePath(ctx context.Context, in temporal.UpdateRunBundlePathInput) error {
	_, err := a.Pool.Exec(ctx, `UPDATE generation_runs SET bundle_path = $1 WHERE id = $2`, in.BundlePath, in.RunID)
	return err
}

// PromoteDeployment copies a run's built bundle to the app's stable live
// prefix and marks the deployments row (created up front by
// AppService.Deploy at status 'deploying') as live.
func (a *PersistActivities) PromoteDeployment(ctx context.Context, in temporal.PromoteDeploymentInput) (temporal.PromoteDeploymentOutput, error) {
	if a.Provisioner == nil {
		return temporal.PromoteDeploymentOutput{}, fmt.Errorf("provisioner not configured")
	}
	if err := a.Provisioner.Promote(ctx, in.AppID, in.RunID); err != nil {
		return temporal.PromoteDeploymentOutput{}, fmt.Errorf("promote bundle: %w", err)
	}

	liveURL := fmt.Sprintf("/api/v1/apps/%s/live/index.html", in.AppID.String())
	_, err := a.Pool.Exec(ctx, `
		UPDATE deployments SET status = 'live', url = $1, deployed_at = now() WHERE id = $2
	`, liveURL, in.DeploymentID)
	if err != nil {
		return temporal.PromoteDeploymentOutput{}, fmt.Errorf("update deployment row: %w", err)
	}
	return temporal.PromoteDeploymentOutput{URL: liveURL}, nil
}

// MarkDeploymentFailed is called from RedeployWorkflow's deferred cleanup
// on any error, so a deployment row never gets stuck at 'deploying'
// forever the way the original, unimplemented deploy stub did.
func (a *PersistActivities) MarkDeploymentFailed(ctx context.Context, in temporal.MarkDeploymentFailedInput) error {
	_, err := a.Pool.Exec(ctx, `UPDATE deployments SET status = 'failed' WHERE id = $1`, in.DeploymentID)
	return err
}
