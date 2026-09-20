package services_test

import (
	"context"
	"testing"

	"github.com/arashrasoulzadeh/appgent/internal/db"
	"github.com/arashrasoulzadeh/appgent/internal/services"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	temporalclient "go.temporal.io/sdk/client"
)

// Note: generateSlug is an unexported helper in internal/services and cannot
// be unit-tested directly from this external test package. Its slug-format
// behavior is still exercised indirectly by TestAppService_Create_Integration
// below, which asserts on app.Slug.

// newTestAppService dials the local dev Temporal server and skips the test
// if it's unavailable, matching the existing Postgres skip pattern below.
func newTestAppService(t *testing.T, pool *pgxpool.Pool) *services.AppService {
	t.Helper()
	tc, err := temporalclient.Dial(temporalclient.Options{HostPort: "localhost:7233"})
	if err != nil {
		t.Skipf("Temporal not available: %v", err)
	}
	t.Cleanup(tc.Close)
	return services.NewAppService(pool, tc)
}

func TestAppService_Create_Integration(t *testing.T) {
	dsn := "postgres://appgent:appgent@localhost:5433/appgent?sslmode=disable"
	ctx := context.Background()

	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}
	defer pool.Close()

	service := newTestAppService(t, pool)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")

	app, run, err := service.Create(ctx, userID, "Test App", "website", "A test app")
	require.NoError(t, err)
	require.NotNil(t, app)
	require.NotNil(t, run)

	assert.Equal(t, "Test App", app.Name)
	assert.Equal(t, "website", app.Kind)
	assert.Equal(t, "generating", app.Status)
	assert.NotEmpty(t, app.Slug)
	assert.True(t, len(app.Slug) > len("test-app-"))

	assert.Equal(t, 1, run.Version)
	assert.Equal(t, "A test app", run.UserPrompt)
	assert.Equal(t, "queued", run.Status)
	assert.NotEmpty(t, run.TemporalWorkflowID)

	// Cleanup
	service.Delete(ctx, userID, app.ID)
}

func TestAppService_List_Integration(t *testing.T) {
	dsn := "postgres://appgent:appgent@localhost:5433/appgent?sslmode=disable"
	ctx := context.Background()

	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}
	defer pool.Close()

	service := newTestAppService(t, pool)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")

	// Create a test app
	app, _, err := service.Create(ctx, userID, "List Test", "website", "Test")
	require.NoError(t, err)
	defer service.Delete(ctx, userID, app.ID)

	apps, err := service.List(ctx, userID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(apps), 1)

	found := false
	for _, a := range apps {
		if a.ID == app.ID {
			found = true
			assert.Equal(t, "List Test", a.Name)
			break
		}
	}
	assert.True(t, found)
}

func TestAppService_GetByID_Integration(t *testing.T) {
	dsn := "postgres://appgent:appgent@localhost:5433/appgent?sslmode=disable"
	ctx := context.Background()

	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}
	defer pool.Close()

	service := newTestAppService(t, pool)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")

	// Create a test app
	app, _, err := service.Create(ctx, userID, "Get Test", "website", "Test")
	require.NoError(t, err)
	defer service.Delete(ctx, userID, app.ID)

	// Get by ID
	gotApp, gotRun, err := service.GetByID(ctx, userID, app.ID)
	require.NoError(t, err)
	assert.Equal(t, app.ID, gotApp.ID)
	assert.Equal(t, app.Name, gotApp.Name)
	assert.NotNil(t, gotRun)
	assert.Equal(t, 1, gotRun.Version)
}

func TestAppService_Delete_Integration(t *testing.T) {
	dsn := "postgres://appgent:appgent@localhost:5433/appgent?sslmode=disable"
	ctx := context.Background()

	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}
	defer pool.Close()

	service := newTestAppService(t, pool)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")

	// Create a test app
	app, _, err := service.Create(ctx, userID, "Delete Test", "website", "Test")
	require.NoError(t, err)

	// Delete
	err = service.Delete(ctx, userID, app.ID)
	require.NoError(t, err)

	// Verify deleted
	_, _, err = service.GetByID(ctx, userID, app.ID)
	assert.Error(t, err)
	assert.Equal(t, services.ErrAppNotFound, err)
}

func TestAppService_Regenerate_Integration(t *testing.T) {
	dsn := "postgres://appgent:appgent@localhost:5433/appgent?sslmode=disable"
	ctx := context.Background()

	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}
	defer pool.Close()

	service := newTestAppService(t, pool)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")

	// Create a test app
	app, run1, err := service.Create(ctx, userID, "Regen Test", "website", "Original prompt")
	require.NoError(t, err)
	defer service.Delete(ctx, userID, app.ID)

	assert.Equal(t, 1, run1.Version)
	assert.Equal(t, "Original prompt", run1.UserPrompt)

	// Regenerate with new prompt
	run2, err := service.Regenerate(ctx, userID, app.ID, "New prompt")
	require.NoError(t, err)

	assert.Equal(t, 2, run2.Version)
	assert.Equal(t, "New prompt", run2.UserPrompt)

	// Regenerate without prompt (should use last)
	run3, err := service.Regenerate(ctx, userID, app.ID, "")
	require.NoError(t, err)

	assert.Equal(t, 3, run3.Version)
	assert.Equal(t, "New prompt", run3.UserPrompt)
}

func TestAppService_GetRuns_Integration(t *testing.T) {
	dsn := "postgres://appgent:appgent@localhost:5433/appgent?sslmode=disable"
	ctx := context.Background()

	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}
	defer pool.Close()

	service := newTestAppService(t, pool)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")

	// Create a test app with multiple runs
	app, _, err := service.Create(ctx, userID, "Runs Test", "website", "Test")
	require.NoError(t, err)
	defer service.Delete(ctx, userID, app.ID)

	// Regenerate a couple times
	service.Regenerate(ctx, userID, app.ID, "Prompt 2")
	service.Regenerate(ctx, userID, app.ID, "Prompt 3")

	runs, err := service.GetRuns(ctx, userID, app.ID)
	require.NoError(t, err)
	assert.Len(t, runs, 3)

	// Should be ordered by version DESC
	assert.Equal(t, 3, runs[0].Version)
	assert.Equal(t, 2, runs[1].Version)
	assert.Equal(t, 1, runs[2].Version)
	assert.Equal(t, "Prompt 3", runs[0].UserPrompt)
	assert.Equal(t, "Prompt 2", runs[1].UserPrompt)
	assert.Equal(t, "Test", runs[2].UserPrompt)
}

// TestAppService_GetRuns_CrossUser_Integration guards against the IDOR bug
// where GetRuns accepted a userID parameter but never used it in the SQL
// query, so any authenticated user could list another user's runs simply by
// knowing (or guessing) the app_id.
func TestAppService_GetRuns_CrossUser_Integration(t *testing.T) {
	dsn := "postgres://appgent:appgent@localhost:5433/appgent?sslmode=disable"
	ctx := context.Background()

	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}
	defer pool.Close()

	service := newTestAppService(t, pool)
	ownerID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	attackerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	app, _, err := service.Create(ctx, ownerID, "Cross User Runs", "website", "Test")
	require.NoError(t, err)
	defer service.Delete(ctx, ownerID, app.ID)

	// The owner can see their own runs.
	runs, err := service.GetRuns(ctx, ownerID, app.ID)
	require.NoError(t, err)
	assert.Len(t, runs, 1)

	// A different user must not be able to read them.
	runs, err = service.GetRuns(ctx, attackerID, app.ID)
	require.NoError(t, err)
	assert.Empty(t, runs, "GetRuns must not leak another user's runs")
}

// TestAppService_GetRunWithSteps_CrossUser_Integration guards against the
// IDOR bug where GetRunWithSteps's SQL query filtered only by run id and
// app id, ignoring the userID parameter entirely.
func TestAppService_GetRunWithSteps_CrossUser_Integration(t *testing.T) {
	dsn := "postgres://appgent:appgent@localhost:5433/appgent?sslmode=disable"
	ctx := context.Background()

	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}
	defer pool.Close()

	service := newTestAppService(t, pool)
	ownerID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	attackerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	app, run, err := service.Create(ctx, ownerID, "Cross User Run Steps", "website", "Test")
	require.NoError(t, err)
	defer service.Delete(ctx, ownerID, app.ID)

	gotRun, _, err := service.GetRunWithSteps(ctx, ownerID, app.ID, run.ID)
	require.NoError(t, err)
	assert.Equal(t, run.ID, gotRun.ID)

	_, _, err = service.GetRunWithSteps(ctx, attackerID, app.ID, run.ID)
	assert.ErrorIs(t, err, services.ErrRunNotFound, "GetRunWithSteps must not leak another user's run")
}

// TestAppService_GetPreviewURL_CrossUser_Integration guards against the IDOR
// bug where GetPreviewURL's SQL query never filtered by the owning user.
func TestAppService_GetPreviewURL_CrossUser_Integration(t *testing.T) {
	dsn := "postgres://appgent:appgent@localhost:5433/appgent?sslmode=disable"
	ctx := context.Background()

	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}
	defer pool.Close()

	service := newTestAppService(t, pool)
	ownerID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	attackerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	app, run, err := service.Create(ctx, ownerID, "Cross User Preview", "website", "Test")
	require.NoError(t, err)
	defer service.Delete(ctx, ownerID, app.ID)

	// No preview_url has been set yet, so the owner gets ErrNoPreview (not
	// ErrRunNotFound) - proving the row itself was found for them.
	_, _, err = service.GetPreviewURL(ctx, ownerID, app.ID, run.ID)
	assert.ErrorIs(t, err, services.ErrNoPreview)

	// A different user must get ErrRunNotFound, i.e. the row must appear
	// invisible to them rather than merely lacking a preview.
	_, _, err = service.GetPreviewURL(ctx, attackerID, app.ID, run.ID)
	assert.ErrorIs(t, err, services.ErrRunNotFound, "GetPreviewURL must not leak another user's run")
}

// TestAppService_CreateDeployment_CrossUser_Integration guards against the
// bug where CreateDeployment took no userID parameter at all and never
// verified that the caller owned the target app before inserting a
// deployment row for it.
func TestAppService_CreateDeployment_CrossUser_Integration(t *testing.T) {
	dsn := "postgres://appgent:appgent@localhost:5433/appgent?sslmode=disable"
	ctx := context.Background()

	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}
	defer pool.Close()

	service := newTestAppService(t, pool)
	ownerID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	attackerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	app, run, err := service.Create(ctx, ownerID, "Cross User Deploy", "website", "Test")
	require.NoError(t, err)
	defer service.Delete(ctx, ownerID, app.ID)

	_, err = service.CreateDeployment(ctx, attackerID, app.ID, run.ID)
	assert.ErrorIs(t, err, services.ErrAppNotFound, "CreateDeployment must reject a non-owner")

	deployment, err := service.CreateDeployment(ctx, ownerID, app.ID, run.ID)
	require.NoError(t, err)
	assert.Equal(t, app.ID, deployment.AppID)
}
