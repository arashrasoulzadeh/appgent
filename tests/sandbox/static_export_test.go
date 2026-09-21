package sandbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/arashrasoulzadeh/appgent/internal/sandbox"
	"github.com/arashrasoulzadeh/appgent/internal/storage"
	"github.com/google/uuid"
)

func TestProvisionerImplementsInterface(t *testing.T) {
	var _ sandbox.Provisioner = (*sandbox.StaticExportProvisioner)(nil)
}

func TestRunPrefixAndAppLivePrefix(t *testing.T) {
	runID := uuid.New()
	if got, want := sandbox.RunPrefix(runID), "runs/"+runID.String()+"/"; got != want {
		t.Errorf("RunPrefix() = %q, want %q", got, want)
	}
	appID := uuid.New()
	if got, want := sandbox.AppLivePrefix(appID), "apps/"+appID.String()+"/"; got != want {
		t.Errorf("AppLivePrefix() = %q, want %q", got, want)
	}
}

// newLiveProvisioner connects to a real MinIO instance for integration
// testing. Unlike the old placeholder storage.Client, NewClient's
// StaticExportProvisioner now performs real network I/O (PutObject,
// ListObjects, CopyObject), so these tests need a reachable MinIO — skip
// rather than fail when one isn't available (e.g. running `go test ./...`
// outside docker compose), matching the pattern used for Postgres-backed
// integration tests in tests/services.
func newLiveProvisioner(t *testing.T) (*sandbox.StaticExportProvisioner, *storage.Client) {
	t.Helper()
	client, err := storage.NewClient("localhost:9000", "minioadmin", "minioadmin", false)
	if err != nil {
		t.Fatalf("storage.NewClient: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.EnsureBucket(ctx, "test-bucket"); err != nil {
		t.Skipf("no reachable MinIO at localhost:9000, skipping integration test: %v", err)
	}
	return sandbox.NewStaticExportProvisioner(client, "test-bucket"), client
}

func TestProvisionAndServe_Integration(t *testing.T) {
	p, client := newLiveProvisioner(t)
	runID := uuid.New()
	files := map[string]string{
		"index.html": "<html></html>",
		"about.html": "<html><body>about</body></html>",
	}

	if err := p.Provision(context.Background(), runID, files); err != nil {
		t.Fatalf("Provision: %v", err)
	}
	defer client.DeletePrefix(context.Background(), "test-bucket", sandbox.RunPrefix(runID))

	obj, err := client.Download(context.Background(), "test-bucket", sandbox.RunPrefix(runID)+"index.html")
	if err != nil {
		t.Fatalf("Download published file: %v", err)
	}
	defer obj.Close()
}

func TestProvisionEmptyFiles_Integration(t *testing.T) {
	p, _ := newLiveProvisioner(t)
	// Provisioning zero files is a no-op, not an error — the run just
	// won't have anything to preview.
	if err := p.Provision(context.Background(), uuid.New(), map[string]string{}); err != nil {
		t.Errorf("Provision with no files should not error: %v", err)
	}
}

func TestPromote_Integration(t *testing.T) {
	p, client := newLiveProvisioner(t)
	appID := uuid.New()
	runID := uuid.New()

	if err := p.Provision(context.Background(), runID, map[string]string{"index.html": "hi"}); err != nil {
		t.Fatalf("Provision: %v", err)
	}
	defer client.DeletePrefix(context.Background(), "test-bucket", sandbox.RunPrefix(runID))

	if err := p.Promote(context.Background(), appID, runID); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	defer client.DeletePrefix(context.Background(), "test-bucket", sandbox.AppLivePrefix(appID))

	obj, err := client.Download(context.Background(), "test-bucket", sandbox.AppLivePrefix(appID)+"index.html")
	if err != nil {
		t.Fatalf("Download promoted file: %v", err)
	}
	defer obj.Close()
}

func TestPromote_NoPublishedRun_Integration(t *testing.T) {
	p, _ := newLiveProvisioner(t)
	// Promoting a run that was never provisioned has nothing to copy —
	// must error, not silently produce an empty "live" deployment.
	if err := p.Promote(context.Background(), uuid.New(), uuid.New()); err == nil {
		t.Error("expected an error promoting a run with no published files")
	}
}

func TestTeardown_Integration(t *testing.T) {
	p, _ := newLiveProvisioner(t)
	if err := p.Teardown(context.Background(), uuid.New()); err != nil {
		t.Errorf("Teardown: unexpected error: %v", err)
	}
}
