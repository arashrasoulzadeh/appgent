package agents

import (
	"context"
	"fmt"

	"github.com/arashrasoulzadeh/appgent/internal/temporal"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PersistActivities holds dependencies (a DB pool) for activities that can't
// be plain functions. Registered on the worker via a bound method so
// GenerateAppWorkflow's persist-on-every-exit-path defer has somewhere to
// actually write the final run/app status.
type PersistActivities struct {
	Pool *pgxpool.Pool
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

	_, err = tx.Exec(ctx, `
		UPDATE generation_runs
		SET status = $1, error = $2, finished_at = now()
		WHERE id = $3
	`, result.Status, errText, runID)
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
