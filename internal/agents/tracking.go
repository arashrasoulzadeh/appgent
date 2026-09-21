package agents

import (
	"context"
	"encoding/json"

	"github.com/arashrasoulzadeh/appgent/internal/logger"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// dbPool is set once at worker startup via SetDBPool. Activities are plain
// functions registered directly on the Temporal worker (not methods on a
// struct), so this is the simplest way to give them DB access without
// reworking how they're registered.
var dbPool *pgxpool.Pool

func SetDBPool(pool *pgxpool.Pool) {
	dbPool = pool
}

// startStep records an agent_steps row in "running" state before an
// activity's real work begins, so the frontend's run-detail timeline can
// show live progress instead of nothing until the whole workflow finishes.
// Returns uuid.Nil if it couldn't record (no pool configured, or a DB
// error) — callers should treat that as "tracking unavailable" and proceed
// with the activity regardless; a missing progress row must never fail
// generation itself.
func startStep(ctx context.Context, runID uuid.UUID, agentType string, attempt int, model string, input interface{}) uuid.UUID {
	if dbPool == nil {
		return uuid.Nil
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		inputJSON = []byte("{}")
	}

	var id uuid.UUID
	err = dbPool.QueryRow(ctx, `
		INSERT INTO agent_steps (run_id, agent_type, attempt, input, model_used, status, started_at)
		VALUES ($1, $2, $3, $4, $5, 'running', now())
		RETURNING id
	`, runID, agentType, attempt, inputJSON, model).Scan(&id)
	if err != nil {
		logger.DefaultLogger.Error("failed to record agent step start", "agent_type", agentType, "error", err)
		return uuid.Nil
	}
	return id
}

// finishStep updates a step recorded by startStep with its outcome.
func finishStep(ctx context.Context, stepID uuid.UUID, output interface{}, stepErr error) {
	if dbPool == nil || stepID == uuid.Nil {
		return
	}

	status := "succeeded"
	var errText *string
	var outputJSON []byte
	if stepErr != nil {
		status = "failed"
		msg := stepErr.Error()
		errText = &msg
	} else {
		outputJSON, _ = json.Marshal(output)
	}

	_, err := dbPool.Exec(ctx, `
		UPDATE agent_steps SET status = $1, output = $2, error = $3, finished_at = now()
		WHERE id = $4
	`, status, outputJSON, errText, stepID)
	if err != nil {
		logger.DefaultLogger.Error("failed to record agent step finish", "step_id", stepID, "error", err)
	}
}
