package services

import (
	"context"
	"testing"

	"github.com/arashrasoulzadeh/appgent/internal/db"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateSlug(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string // we can't check exact because of random suffix, but we can check format
	}{
		{"simple", "My App", "my-app-"},
		{"with spaces", "  Hello World  ", "hello-world-"},
		{"special chars", "App@#$%^&*()", "app-"},
		{"empty after cleanup", "!@#$%", "app-"},
		{"multiple hyphens", "a---b---c", "a-b-c-"},
		{"unicode", "café", "caf-"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := generateSlug(tc.input)
			assert.True(t, len(result) > len(tc.expected), "result: %s", result)
			assert.True(t, len(result) <= 50, "slug too long: %s", result)
			// Check it ends with 6-char suffix
			assert.Regexp(t, `^.+-[a-f0-9]{6}$`, result)
			// Check prefix matches expected
			if tc.expected != "" {
				assert.True(t, len(result) >= len(tc.expected))
			}
		})
	}
}

func TestGenerateSlug_Unique(t *testing.T) {
	slugs := make(map[string]bool)
	for i := 0; i < 100; i++ {
		slug := generateSlug("Test App")
		assert.False(t, slugs[slug], "duplicate slug: %s", slug)
		slugs[slug] = true
	}
}

func TestAppService_Create_Integration(t *testing.T) {
	dsn := "postgres://appgent:appgent@localhost:5433/appgent?sslmode=disable"
	ctx := context.Background()

	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}
	defer pool.Close()

	service := NewAppService(pool)
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

	service := NewAppService(pool)
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

	service := NewAppService(pool)
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

	service := NewAppService(pool)
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
	assert.Equal(t, ErrAppNotFound, err)
}

func TestAppService_Regenerate_Integration(t *testing.T) {
	dsn := "postgres://appgent:appgent@localhost:5433/appgent?sslmode=disable"
	ctx := context.Background()

	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}
	defer pool.Close()

	service := NewAppService(pool)
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

	service := NewAppService(pool)
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