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

// fakeBuilder passes files through unchanged by default (buildFn nil) so
// tests that only care about the provisioner side don't need real build
// output; set buildFn to simulate a build transforming source into static
// output, or to simulate a build failure.
type fakeBuilder struct {
	calls   []map[string]string
	buildFn func(files map[string]string) (map[string]string, string, error)
}

func (f *fakeBuilder) Build(_ context.Context, _ uuid.UUID, files map[string]string) (map[string]string, string, error) {
	f.calls = append(f.calls, files)
	if f.buildFn != nil {
		return f.buildFn(files)
	}
	return files, "", nil
}

func setFakes(t *testing.T, p *fakeProvisioner, b *fakeBuilder) {
	t.Helper()
	agents.SetProvisioner(p)
	agents.SetBuilder(b)
	t.Cleanup(func() {
		agents.SetProvisioner(nil)
		agents.SetBuilder(nil)
	})
}

// TestPublishBundleActivity_NoProvisionerConfigured is a regression test:
// the worker must fail loudly if SetProvisioner was never called, rather
// than silently doing nothing and leaving every run's bundle_path NULL
// forever (the original bug this whole activity exists to fix).
func TestPublishBundleActivity_NoProvisionerConfigured(t *testing.T) {
	agents.SetProvisioner(nil)
	agents.SetBuilder(&fakeBuilder{})
	t.Cleanup(func() { agents.SetBuilder(nil) })

	_, err := agents.PublishBundleActivity(context.Background(), apptemporal.PublishBundleInput{
		RunID: uuid.New(),
		Files: map[string]string{"index.html": "x"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provisioner not configured")
}

// TestPublishBundleActivity_NoBuilderConfigured is a regression test for
// the follow-up bug: even with a provisioner configured, publishing must
// not fall back to uploading raw .tsx/.ts source when no builder is wired
// up — that's how deployed apps ended up 404ing (no index.html, since
// nothing ever compiled the source into static HTML).
func TestPublishBundleActivity_NoBuilderConfigured(t *testing.T) {
	agents.SetProvisioner(&fakeProvisioner{})
	agents.SetBuilder(nil)
	t.Cleanup(func() { agents.SetProvisioner(nil) })

	_, err := agents.PublishBundleActivity(context.Background(), apptemporal.PublishBundleInput{
		RunID: uuid.New(),
		Files: map[string]string{"index.html": "x"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "builder not configured")
}

func TestPublishBundleActivity_NoFiles(t *testing.T) {
	fp := &fakeProvisioner{}
	fb := &fakeBuilder{}
	setFakes(t, fp, fb)

	_, err := agents.PublishBundleActivity(context.Background(), apptemporal.PublishBundleInput{
		RunID: uuid.New(),
		Files: map[string]string{},
	})
	require.Error(t, err)
	assert.Empty(t, fp.provisionCalls, "must not call Provision with zero files")
	assert.Empty(t, fb.calls, "must not call Build with zero files")
}

func TestPublishBundleActivity_Success(t *testing.T) {
	fp := &fakeProvisioner{}
	builtFiles := map[string]string{"index.html": "<html>built</html>"}
	fb := &fakeBuilder{buildFn: func(map[string]string) (map[string]string, string, error) { return builtFiles, "npm install...\nbuild succeeded", nil }}
	setFakes(t, fp, fb)

	runID := uuid.New()
	sourceFiles := map[string]string{"src/app/page.tsx": "export default function Page() {}"}
	out, err := agents.PublishBundleActivity(context.Background(), apptemporal.PublishBundleInput{
		RunID: runID,
		Files: sourceFiles,
	})
	require.NoError(t, err)
	assert.Equal(t, "runs/"+runID.String()+"/", out.BundlePath)
	require.Len(t, fb.calls, 1)
	assert.Equal(t, sourceFiles, fb.calls[0], "builder must receive the raw source")
	require.Len(t, fp.provisionCalls, 1)
	assert.Equal(t, builtFiles, fp.provisionCalls[0], "provisioner must receive the BUILT output, not raw source")
}

func TestPublishBundleActivity_BuildError(t *testing.T) {
	fp := &fakeProvisioner{}
	fb := &fakeBuilder{buildFn: func(map[string]string) (map[string]string, string, error) {
		return nil, "npm ERR! ...", fmt.Errorf("npm run build failed: exit code 1")
	}}
	setFakes(t, fp, fb)

	_, err := agents.PublishBundleActivity(context.Background(), apptemporal.PublishBundleInput{
		RunID: uuid.New(),
		Files: map[string]string{"src/app/page.tsx": "x"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "npm run build failed")
	assert.Empty(t, fp.provisionCalls, "must not publish anything when the build fails")
}

func TestPublishBundleActivity_ProvisionError(t *testing.T) {
	fp := &fakeProvisioner{provisionErr: fmt.Errorf("upload failed")}
	fb := &fakeBuilder{}
	setFakes(t, fp, fb)

	_, err := agents.PublishBundleActivity(context.Background(), apptemporal.PublishBundleInput{
		RunID: uuid.New(),
		Files: map[string]string{"index.html": "x"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload failed")
}
