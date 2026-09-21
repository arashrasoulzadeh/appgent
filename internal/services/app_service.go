package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	apptemporal "github.com/arashrasoulzadeh/appgent/internal/temporal"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	temporalclient "go.temporal.io/sdk/client"
)

const generationTaskQueue = "appgent-generation"

// nullString/nullTime/nullInt64 marshal sql.Null* as a plain value or null,
// instead of Go's default {"String":"...","Valid":true} struct encoding.
func nullString(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

func nullTime(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	return &v.Time
}

func nullInt64(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}

type App struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"-"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Run struct {
	ID                 uuid.UUID      `json:"id"`
	AppID              uuid.UUID      `json:"app_id"`
	Version            int            `json:"version"`
	UserPrompt         string         `json:"user_prompt"`
	Status             string         `json:"status"`
	TemporalWorkflowID string         `json:"-"`
	BundlePath         sql.NullString `json:"-"`
	PreviewURL         sql.NullString `json:"-"`
	Error              sql.NullString `json:"-"`
	StartedAt          sql.NullTime   `json:"-"`
	FinishedAt         sql.NullTime   `json:"-"`
	CreatedAt          time.Time      `json:"created_at"`
}

func (r *Run) MarshalJSON() ([]byte, error) {
	type alias Run
	return json.Marshal(struct {
		*alias
		BundlePath string     `json:"bundle_path,omitempty"`
		PreviewURL string     `json:"preview_url,omitempty"`
		Error      *string    `json:"error,omitempty"`
		StartedAt  *time.Time `json:"started_at,omitempty"`
		FinishedAt *time.Time `json:"finished_at,omitempty"`
	}{
		alias:      (*alias)(r),
		BundlePath: r.BundlePath.String,
		PreviewURL: r.PreviewURL.String,
		Error:      nullString(r.Error),
		StartedAt:  nullTime(r.StartedAt),
		FinishedAt: nullTime(r.FinishedAt),
	})
}

type AgentStep struct {
	ID         uuid.UUID      `json:"-"`
	RunID      uuid.UUID      `json:"-"`
	AgentType  string         `json:"agent_type"`
	Attempt    int            `json:"attempt"`
	Input      []byte         `json:"-"`
	Output     []byte         `json:"-"`
	ModelUsed  string         `json:"model_used"`
	TokensUsed sql.NullInt64  `json:"-"`
	Status     string         `json:"status"`
	Error      sql.NullString `json:"-"`
	StartedAt  sql.NullTime   `json:"-"`
	FinishedAt sql.NullTime   `json:"-"`
	CreatedAt  time.Time      `json:"-"`
}

func (s *AgentStep) MarshalJSON() ([]byte, error) {
	type alias AgentStep
	var summary string
	if len(s.Output) > 0 {
		summary = string(s.Output)
		if len(summary) > 300 {
			summary = summary[:300] + "..."
		}
	}
	return json.Marshal(struct {
		*alias
		TokensUsed    *int64     `json:"tokens_used,omitempty"`
		Error         *string    `json:"error,omitempty"`
		StartedAt     *time.Time `json:"started_at,omitempty"`
		FinishedAt    *time.Time `json:"finished_at,omitempty"`
		OutputSummary string     `json:"output_summary,omitempty"`
	}{
		alias:         (*alias)(s),
		TokensUsed:    nullInt64(s.TokensUsed),
		Error:         nullString(s.Error),
		StartedAt:     nullTime(s.StartedAt),
		FinishedAt:    nullTime(s.FinishedAt),
		OutputSummary: summary,
	})
}

type Deployment struct {
	ID         uuid.UUID      `json:"id"`
	AppID      uuid.UUID      `json:"-"`
	RunID      uuid.UUID      `json:"-"`
	URL        sql.NullString `json:"-"`
	Status     string         `json:"status"`
	DeployedAt sql.NullTime   `json:"-"`
	CreatedAt  time.Time      `json:"created_at"`
}

func (d *Deployment) MarshalJSON() ([]byte, error) {
	type alias Deployment
	return json.Marshal(struct {
		*alias
		URL        string     `json:"url,omitempty"`
		DeployedAt *time.Time `json:"deployed_at,omitempty"`
	}{
		alias:      (*alias)(d),
		URL:        d.URL.String,
		DeployedAt: nullTime(d.DeployedAt),
	})
}

type User struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
}

type AppService struct {
	pool     *pgxpool.Pool
	temporal temporalclient.Client
}

func NewAppService(pool *pgxpool.Pool, temporal temporalclient.Client) *AppService {
	return &AppService{pool: pool, temporal: temporal}
}

var ErrUserNotFound = errors.New("user not found")

func (s *AppService) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	user := &User{}
	err := s.pool.QueryRow(ctx, "SELECT id, email, password_hash FROM users WHERE email = $1", email).
		Scan(&user.ID, &user.Email, &user.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (s *AppService) Create(ctx context.Context, userID uuid.UUID, name, kind, prompt string) (*App, *Run, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)

	var appCount int
	if err := tx.QueryRow(ctx, "SELECT COUNT(*) FROM apps WHERE user_id = $1", userID).Scan(&appCount); err != nil {
		return nil, nil, err
	}
	if appCount >= MaxAppsPerUser {
		return nil, nil, ErrAppLimitReached
	}

	slug := generateSlug(name)
	for {
		var exists bool
		err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM apps WHERE slug = $1)", slug).Scan(&exists)
		if err != nil {
			return nil, nil, err
		}
		if !exists {
			break
		}
		slug = generateSlug(name)
	}

	app := &App{}
	err = tx.QueryRow(ctx, `
		INSERT INTO apps (user_id, name, slug, kind, status)
		VALUES ($1, $2, $3, $4, 'generating')
		RETURNING id, user_id, name, slug, kind, status, created_at, updated_at
	`, userID, name, slug, kind).Scan(
		&app.ID, &app.UserID, &app.Name, &app.Slug, &app.Kind, &app.Status, &app.CreatedAt, &app.UpdatedAt,
	)
	if err != nil {
		return nil, nil, err
	}

	runID := uuid.New()
	workflowID := "run-" + runID.String()

	run := &Run{}
	err = tx.QueryRow(ctx, `
		INSERT INTO generation_runs (id, app_id, version, user_prompt, status, temporal_workflow_id)
		VALUES ($1, $2, 1, $3, 'queued', $4)
		RETURNING id, app_id, version, user_prompt, status, temporal_workflow_id, bundle_path, preview_url, error, started_at, finished_at, created_at
	`, runID, app.ID, prompt, workflowID).Scan(
		&run.ID, &run.AppID, &run.Version, &run.UserPrompt, &run.Status, &run.TemporalWorkflowID,
		&run.BundlePath, &run.PreviewURL, &run.Error, &run.StartedAt, &run.FinishedAt, &run.CreatedAt,
	)
	if err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}

	_, err = s.temporal.ExecuteWorkflow(ctx, temporalclient.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: generationTaskQueue,
	}, apptemporal.GenerateAppWorkflow, apptemporal.GenerateAppInput{
		RunID:      run.ID,
		AppID:      app.ID,
		AppKind:    app.Kind,
		UserPrompt: prompt,
	})
	if err != nil {
		// The DB rows were already committed; mark them failed instead of
		// leaving the app/run stuck in "generating"/"queued" forever.
		s.pool.Exec(ctx, `UPDATE generation_runs SET status = 'failed', error = $2 WHERE id = $1`, run.ID, err.Error())
		s.pool.Exec(ctx, `UPDATE apps SET status = 'failed', updated_at = now() WHERE id = $1`, app.ID)
		return nil, nil, err
	}

	return app, run, nil
}

func (s *AppService) GetByID(ctx context.Context, userID, appID uuid.UUID) (*App, *Run, error) {
	app := &App{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, name, slug, kind, status, created_at, updated_at
		FROM apps WHERE id = $1 AND user_id = $2
	`, appID, userID).Scan(
		&app.ID, &app.UserID, &app.Name, &app.Slug, &app.Kind, &app.Status, &app.CreatedAt, &app.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrAppNotFound
		}
		return nil, nil, err
	}

	run := &Run{}
	err = s.pool.QueryRow(ctx, `
		SELECT id, app_id, version, user_prompt, status, temporal_workflow_id, bundle_path, preview_url, error, started_at, finished_at, created_at
		FROM generation_runs WHERE app_id = $1 ORDER BY version DESC LIMIT 1
	`, appID).Scan(
		&run.ID, &run.AppID, &run.Version, &run.UserPrompt, &run.Status, &run.TemporalWorkflowID,
		&run.BundlePath, &run.PreviewURL, &run.Error, &run.StartedAt, &run.FinishedAt, &run.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return app, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}

	return app, run, nil
}

func (s *AppService) List(ctx context.Context, userID uuid.UUID) ([]*App, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, name, slug, kind, status, created_at, updated_at
		FROM apps WHERE user_id = $1 ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []*App
	for rows.Next() {
		app := &App{}
		err := rows.Scan(&app.ID, &app.UserID, &app.Name, &app.Slug, &app.Kind, &app.Status, &app.CreatedAt, &app.UpdatedAt)
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}
	return apps, rows.Err()
}

func (s *AppService) Delete(ctx context.Context, userID, appID uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM apps WHERE id = $1 AND user_id = $2`, appID, userID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrAppNotFound
	}
	return nil
}

func (s *AppService) Regenerate(ctx context.Context, userID, appID uuid.UUID, prompt string) (*Run, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var appName, appKind string
	err = tx.QueryRow(ctx, `SELECT name, kind FROM apps WHERE id = $1 AND user_id = $2`, appID, userID).Scan(&appName, &appKind)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAppNotFound
		}
		return nil, err
	}

	if prompt == "" {
		err = tx.QueryRow(ctx, `SELECT user_prompt FROM generation_runs WHERE app_id = $1 ORDER BY version DESC LIMIT 1`, appID).Scan(&prompt)
		if err != nil {
			return nil, err
		}
	}

	var version int
	err = tx.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) + 1 FROM generation_runs WHERE app_id = $1`, appID).Scan(&version)
	if err != nil {
		return nil, err
	}

	runID := uuid.New()
	workflowID := "run-" + runID.String()

	run := &Run{}
	err = tx.QueryRow(ctx, `
		INSERT INTO generation_runs (id, app_id, version, user_prompt, status, temporal_workflow_id)
		VALUES ($1, $2, $3, $4, 'queued', $5)
		RETURNING id, app_id, version, user_prompt, status, temporal_workflow_id, bundle_path, preview_url, error, started_at, finished_at, created_at
	`, runID, appID, version, prompt, workflowID).Scan(
		&run.ID, &run.AppID, &run.Version, &run.UserPrompt, &run.Status, &run.TemporalWorkflowID,
		&run.BundlePath, &run.PreviewURL, &run.Error, &run.StartedAt, &run.FinishedAt, &run.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(ctx, `UPDATE apps SET status = 'generating', updated_at = now() WHERE id = $1`, appID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	_, err = s.temporal.ExecuteWorkflow(ctx, temporalclient.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: generationTaskQueue,
	}, apptemporal.GenerateAppWorkflow, apptemporal.GenerateAppInput{
		RunID:      run.ID,
		AppID:      appID,
		AppKind:    appKind,
		UserPrompt: prompt,
	})
	if err != nil {
		// The DB rows were already committed; mark them failed instead of
		// leaving the app/run stuck in "generating"/"queued" forever.
		s.pool.Exec(ctx, `UPDATE generation_runs SET status = 'failed', error = $2 WHERE id = $1`, run.ID, err.Error())
		s.pool.Exec(ctx, `UPDATE apps SET status = 'failed', updated_at = now() WHERE id = $1`, appID)
		return nil, err
	}

	return run, nil
}

func (s *AppService) GetRuns(ctx context.Context, userID, appID uuid.UUID) ([]*Run, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.id, r.app_id, r.version, r.user_prompt, r.status, r.temporal_workflow_id, r.bundle_path, r.preview_url, r.error, r.started_at, r.finished_at, r.created_at
		FROM generation_runs r
		JOIN apps a ON r.app_id = a.id
		WHERE r.app_id = $1 AND a.user_id = $2
		ORDER BY r.version DESC
	`, appID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []*Run
	for rows.Next() {
		run := &Run{}
		err := rows.Scan(&run.ID, &run.AppID, &run.Version, &run.UserPrompt, &run.Status, &run.TemporalWorkflowID,
			&run.BundlePath, &run.PreviewURL, &run.Error, &run.StartedAt, &run.FinishedAt, &run.CreatedAt)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (s *AppService) GetRunWithSteps(ctx context.Context, userID, appID, runID uuid.UUID) (*Run, []*AgentStep, error) {
	run := &Run{}
	err := s.pool.QueryRow(ctx, `
		SELECT r.id, r.app_id, r.version, r.user_prompt, r.status, r.temporal_workflow_id, r.bundle_path, r.preview_url, r.error, r.started_at, r.finished_at, r.created_at
		FROM generation_runs r
		JOIN apps a ON r.app_id = a.id
		WHERE r.id = $1 AND r.app_id = $2 AND a.user_id = $3
	`, runID, appID, userID).Scan(
		&run.ID, &run.AppID, &run.Version, &run.UserPrompt, &run.Status, &run.TemporalWorkflowID,
		&run.BundlePath, &run.PreviewURL, &run.Error, &run.StartedAt, &run.FinishedAt, &run.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrRunNotFound
		}
		return nil, nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, run_id, agent_type, attempt, input, output, model_used, tokens_used, status, error, started_at, finished_at, created_at
		FROM agent_steps WHERE run_id = $1 ORDER BY attempt, agent_type
	`, runID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var steps []*AgentStep
	for rows.Next() {
		step := &AgentStep{}
		err := rows.Scan(&step.ID, &step.RunID, &step.AgentType, &step.Attempt, &step.Input, &step.Output,
			&step.ModelUsed, &step.TokensUsed, &step.Status, &step.Error, &step.StartedAt, &step.FinishedAt, &step.CreatedAt)
		if err != nil {
			return nil, nil, err
		}
		steps = append(steps, step)
	}

	return run, steps, rows.Err()
}

func (s *AppService) CreateDeployment(ctx context.Context, userID, appID, runID uuid.UUID) (*Deployment, error) {
	var owns bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM apps WHERE id = $1 AND user_id = $2)`, appID, userID).Scan(&owns); err != nil {
		return nil, err
	}
	if !owns {
		return nil, ErrAppNotFound
	}

	deployment := &Deployment{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO deployments (app_id, run_id, status)
		VALUES ($1, $2, 'deploying')
		RETURNING id, app_id, run_id, url, status, deployed_at, created_at
	`, appID, runID).Scan(
		&deployment.ID, &deployment.AppID, &deployment.RunID, &deployment.URL, &deployment.Status, &deployment.DeployedAt, &deployment.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return deployment, nil
}

func (s *AppService) GetDeployments(ctx context.Context, userID, appID uuid.UUID) ([]*Deployment, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT d.id, d.app_id, d.run_id, d.url, d.status, d.deployed_at, d.created_at
		FROM deployments d
		JOIN apps a ON d.app_id = a.id
		WHERE a.user_id = $1 AND d.app_id = $2
		ORDER BY d.created_at DESC
	`, userID, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deployments []*Deployment
	for rows.Next() {
		d := &Deployment{}
		err := rows.Scan(&d.ID, &d.AppID, &d.RunID, &d.URL, &d.Status, &d.DeployedAt, &d.CreatedAt)
		if err != nil {
			return nil, err
		}
		deployments = append(deployments, d)
	}
	return deployments, rows.Err()
}

func (s *AppService) GetPreviewURL(ctx context.Context, userID, appID, runID uuid.UUID) (string, time.Time, error) {
	var previewURL sql.NullString
	var expiresAt time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT r.preview_url
		FROM generation_runs r
		JOIN apps a ON r.app_id = a.id
		WHERE r.id = $1 AND r.app_id = $2 AND a.user_id = $3
	`, runID, appID, userID).Scan(&previewURL)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", time.Time{}, ErrRunNotFound
		}
		return "", time.Time{}, err
	}
	if !previewURL.Valid {
		return "", time.Time{}, ErrNoPreview
	}
	expiresAt = time.Now().Add(1 * time.Hour)
	return previewURL.String, expiresAt, nil
}

var (
	ErrAppNotFound     = errors.New("app not found")
	ErrRunNotFound     = errors.New("run not found")
	ErrNoPreview       = errors.New("preview not available")
	ErrAppLimitReached = errors.New("app limit reached")
)

// MaxAppsPerUser caps how many apps a single user can create.
const MaxAppsPerUser = 10

func generateSlug(name string) string {
	base := strings.ToLower(strings.TrimSpace(name))
	base = regexp.MustCompile(`[^\w\s-]`).ReplaceAllString(base, "")
	base = regexp.MustCompile(`[\s_-]+`).ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if base == "" {
		base = "app"
	}
	suffix := uuid.New().String()[:6]
	return base + "-" + suffix
}
