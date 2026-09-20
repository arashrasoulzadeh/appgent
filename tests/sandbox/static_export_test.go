package sandbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/arashrasoulzadeh/appgent/internal/sandbox"
	"github.com/arashrasoulzadeh/appgent/internal/storage"
	"github.com/google/uuid"
)

// storage.NewClient's current implementation is a placeholder that never
// makes a real network call (see internal/storage/storage.go), so
// constructing one here and exercising StaticExportProvisioner against it is
// safe and involves no live infra.
func newTestProvisioner(t *testing.T) *sandbox.StaticExportProvisioner {
	t.Helper()
	client, err := storage.NewClient("localhost:9000", "access", "secret", false)
	if err != nil {
		t.Fatalf("storage.NewClient: %v", err)
	}
	return sandbox.NewStaticExportProvisioner(client, "test-bucket", "https://preview.example.com", "apps.example.com")
}

func TestProvisionerImplementsInterface(t *testing.T) {
	var _ sandbox.Provisioner = (*sandbox.StaticExportProvisioner)(nil)
}

// TestProvision is a regression test for a bug where Upload was called with
// a nil io.Reader instead of the actual file content (strings.NewReader),
// which would silently drop every generated file's contents.
func TestProvision(t *testing.T) {
	p := newTestProvisioner(t)
	runID := uuid.New()
	files := map[string]string{
		"index.html": "<html></html>",
		"about.html": "<html><body>about</body></html>",
	}

	previewURL, expiresAt, err := p.Provision(context.Background(), runID, files)
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}

	wantURL := "https://preview.example.com/runs/" + runID.String() + "/index.html"
	if previewURL != wantURL {
		t.Errorf("previewURL = %q, want %q", previewURL, wantURL)
	}
	if !expiresAt.After(time.Now()) {
		t.Errorf("expiresAt = %v, want a time in the future", expiresAt)
	}
	if expiresAt.After(time.Now().Add(2 * time.Hour)) {
		t.Errorf("expiresAt = %v, want within ~1 hour", expiresAt)
	}
}

func TestProvisionEmptyFiles(t *testing.T) {
	p := newTestProvisioner(t)
	runID := uuid.New()

	previewURL, _, err := p.Provision(context.Background(), runID, map[string]string{})
	if err != nil {
		t.Fatalf("Provision with no files should not error: %v", err)
	}
	if previewURL == "" {
		t.Error("expected a non-empty preview URL even with no files")
	}
}

func TestPromote(t *testing.T) {
	p := newTestProvisioner(t)
	appID := uuid.New()
	runID := uuid.New()

	liveURL, err := p.Promote(context.Background(), appID, runID)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}

	wantPrefix := "https://" + appID.String()[:8] + ".apps.example.com"
	if liveURL != wantPrefix {
		t.Errorf("liveURL = %q, want %q", liveURL, wantPrefix)
	}
}

func TestTeardown(t *testing.T) {
	p := newTestProvisioner(t)
	if err := p.Teardown(context.Background(), uuid.New()); err != nil {
		t.Errorf("Teardown: unexpected error: %v", err)
	}
}
