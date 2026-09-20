package services

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type App struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Name      string
	Slug      string
	Kind      string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Run struct {
	ID                uuid.UUID
	AppID             uuid.UUID
	Version           int
	UserPrompt        string
	Status            string
	TemporalWorkflowID string
	BundlePath        sql.NullString
	PreviewURL        sql.NullString
	Error             sql.NullString
	StartedAt         sql.NullTime
	FinishedAt        sql.NullTime
	CreatedAt         time.Time
}

type AgentStep struct {
	ID          uuid.UUID
	RunID       uuid.UUID
	AgentType   string
	Attempt     int
	Input       []byte
	Output      []byte
	ModelUsed   string
	TokensUsed  sql.NullInt64
	Status      string
	Error       sql.NullString
	StartedAt   sql.NullTime
	FinishedAt  sql.NullTime
	CreatedAt   time.Time
}

type Deployment struct {
	ID          uuid.UUID
	AppID       uuid.UUID
	RunID       uuid.UUID
	URL         sql.NullString
	Status      string
	DeployedAt  sql.NullTime
	CreatedAt   time.Time
}

type User struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
}

type AppService struct {
	pool *pgxpool.Pool
}

func NewAppService(pool *pgxpool.Pool) *AppService {
	return &AppService{pool: pool}
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

	run := &Run{}
	err = tx.QueryRow(ctx, `
		INSERT INTO generation_runs (app_id, version, user_prompt, status, temporal_workflow_id)
		VALUES ($1, 1, $2, 'queued', $3)
		RETURNING id, app_id, version, user_prompt, status, temporal_workflow_id, bundle_path, preview_url, error, started_at, finished_at, created_at
	`, app.ID, prompt, uuid.New().String()).Scan(
		&run.ID, &run.AppID, &run.Version, &run.UserPrompt, &run.Status, &run.TemporalWorkflowID,
		&run.BundlePath, &run.PreviewURL, &run.Error, &run.StartedAt, &run.FinishedAt, &run.CreatedAt,
	)
	if err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(ctx); err != nil {
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
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
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

	var appName string
	err = tx.QueryRow(ctx, `SELECT name FROM apps WHERE id = $1 AND user_id = $2`, appID, userID).Scan(&appName)
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

	run := &Run{}
	err = tx.QueryRow(ctx, `
		INSERT INTO generation_runs (app_id, version, user_prompt, status, temporal_workflow_id)
		VALUES ($1, $2, $3, 'queued', $4)
		RETURNING id, app_id, version, user_prompt, status, temporal_workflow_id, bundle_path, preview_url, error, started_at, finished_at, created_at
	`, appID, version, prompt, uuid.New().String()).Scan(
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

	return run, nil
}

func (s *AppService) GetRuns(ctx context.Context, userID, appID uuid.UUID) ([]*Run, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, app_id, version, user_prompt, status, temporal_workflow_id, bundle_path, preview_url, error, started_at, finished_at, created_at
		FROM generation_runs WHERE app_id = $1 ORDER BY version DESC
	`, appID)
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
		SELECT id, app_id, version, user_prompt, status, temporal_workflow_id, bundle_path, preview_url, error, started_at, finished_at, created_at
		FROM generation_runs WHERE id = $1 AND app_id = $2
	`, runID, appID).Scan(
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

func (s *AppService) CreateDeployment(ctx context.Context, appID, runID uuid.UUID) (*Deployment, error) {
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
		SELECT preview_url FROM generation_runs WHERE id = $1 AND app_id = $2
	`, runID, appID).Scan(&previewURL)
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
	ErrAppNotFound   = errors.New("app not found")
	ErrRunNotFound   = errors.New("run not found")
	ErrNoPreview     = errors.New("preview not available")
)

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