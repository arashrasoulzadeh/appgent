package agents_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/arashrasoulzadeh/appgent/internal/agents"
	apptemporal "github.com/arashrasoulzadeh/appgent/internal/temporal"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeProvisioner struct {
	provisionCalls []map[string]string
	provisionErr   error
}

func (f *fakeProvisioner) Provision(_ context.Context, _ uuid.UUID, files map[string]string) error {
	f.provisionCalls = append(f.provisionCalls, files)
	return f.provisionErr
}
func (f *fakeProvisioner) Promote(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (f *fakeProvisioner) Teardown(context.Context, uuid.UUID) error          { return nil }

// TestPublishBundleActivity_NoProvisionerConfigured is a regression test:
// the worker must fail loudly if SetProvisioner was never called, rather
// than silently doing nothing and leaving every run's bundle_path NULL
// forever (the original bug this whole activity exists to fix).
func TestPublishBundleActivity_NoProvisionerConfigured(t *testing.T) {
	agents.SetProvisioner(nil)
	_, err := agents.PublishBundleActivity(context.Background(), apptemporal.PublishBundleInput{
		RunID: uuid.New(),
		Files: map[string]string{"index.html": "x"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provisioner not configured")
}

func TestPublishBundleActivity_NoFiles(t *testing.T) {
	fp := &fakeProvisioner{}
	agents.SetProvisioner(fp)
	t.Cleanup(func() { agents.SetProvisioner(nil) })

	_, err := agents.PublishBundleActivity(context.Background(), apptemporal.PublishBundleInput{
		RunID: uuid.New(),
		Files: map[string]string{},
	})
	require.Error(t, err)
	assert.Empty(t, fp.provisionCalls, "must not call Provision with zero files")
}

func TestPublishBundleActivity_Success(t *testing.T) {
	fp := &fakeProvisioner{}
	agents.SetProvisioner(fp)
	t.Cleanup(func() { agents.SetProvisioner(nil) })

	runID := uuid.New()
	files := map[string]string{"index.html": "<html></html>"}
	out, err := agents.PublishBundleActivity(context.Background(), apptemporal.PublishBundleInput{
		RunID: runID,
		Files: files,
	})
	require.NoError(t, err)
	assert.Equal(t, "runs/"+runID.String()+"/", out.BundlePath)
	require.Len(t, fp.provisionCalls, 1)
	assert.Equal(t, files, fp.provisionCalls[0])
}

func TestPublishBundleActivity_ProvisionError(t *testing.T) {
	fp := &fakeProvisioner{provisionErr: fmt.Errorf("upload failed")}
	agents.SetProvisioner(fp)
	t.Cleanup(func() { agents.SetProvisioner(nil) })

	_, err := agents.PublishBundleActivity(context.Background(), apptemporal.PublishBundleInput{
		RunID: uuid.New(),
		Files: map[string]string{"index.html": "x"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload failed")
}
