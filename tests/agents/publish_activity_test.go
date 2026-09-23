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
	provisionCalls       []map[string]string
	provisionErr         error
	provisionSourceCalls []map[string]string
	provisionSourceErr   error
	fetchSourceFiles     map[string]string
	fetchSourceErr       error
}

func (f *fakeProvisioner) Provision(_ context.Context, _ uuid.UUID, files map[string]string) error {
	f.provisionCalls = append(f.provisionCalls, files)
	return f.provisionErr
}
func (f *fakeProvisioner) ProvisionSource(_ context.Context, _ uuid.UUID, files map[string]string) error {
	f.provisionSourceCalls = append(f.provisionSourceCalls, files)
	return f.provisionSourceErr
}
func (f *fakeProvisioner) FetchSource(context.Context, uuid.UUID) (map[string]string, error) {
	return f.fetchSourceFiles, f.fetchSourceErr
}
func (f *fakeProvisioner) Promote(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (f *fakeProvisioner) Teardown(context.Context, uuid.UUID) error           { return nil }

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
	fb := &fakeBuilder{buildFn: func(map[string]string) (map[string]string, string, error) {
		return builtFiles, "npm install...\nbuild succeeded", nil
	}}
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

// TestPublishBundleActivity_ForcesTsConfigPathAlias_BeforeBuild is a
// regression test for the ACTUAL root cause behind every "Module not
// found" build failure chased across production runs (v27, v28, v30,
// v31): the LLM generated a tsconfig.json with no "baseUrl"/"paths"
// mapping for the "@/*" alias every generated file's imports rely on
// (e.g. "@/components/Header"). Without it, Next.js's webpack build can't
// resolve ANY "@/..." import — producing the exact same "Module not
// found: Can't resolve '@/components/X'" error a genuinely missing
// component file would, even though the component files were confirmed
// present (verified directly against production's agent_steps.input JSON
// for the exact run that hit this). PublishBundleActivity is what BOTH
// GenerateAppWorkflow's final publish step AND RedeployWorkflow's
// rebuild-from-saved-source path call, so it must force-correct
// tsconfig.json before every real build, regardless of what the LLM
// wrote or when the source was originally generated.
func TestPublishBundleActivity_ForcesTsConfigPathAlias_BeforeBuild(t *testing.T) {
	fp := &fakeProvisioner{}
	fb := &fakeBuilder{buildFn: func(files map[string]string) (map[string]string, string, error) {
		return files, "", nil
	}}
	setFakes(t, fp, fb)

	brokenTsConfig := `{"compilerOptions":{"target":"es5","strict":true}}`
	_, err := agents.PublishBundleActivity(context.Background(), apptemporal.PublishBundleInput{
		RunID: uuid.New(),
		Files: map[string]string{
			"tsconfig.json":             brokenTsConfig,
			"src/app/page.tsx":          `import Header from "@/components/Header"`,
			"src/components/Header.tsx": "export default function Header() { return null }",
		},
	})
	require.NoError(t, err)
	require.Len(t, fb.calls, 1)
	gotTsConfig := fb.calls[0]["tsconfig.json"]
	assert.NotEqual(t, brokenTsConfig, gotTsConfig, "tsconfig.json was sent to the build unmodified — the missing @/* path alias was never corrected")
	assert.Contains(t, gotTsConfig, `"@/*"`, "corrected tsconfig.json still missing the @/* path alias")
}

// TestPublishBundleActivity_ForcesNextConfigBasePath is a regression test
// for a real production failure reported right after a successful deploy:
// the app served correctly at index.html, but EVERY /_next/static/*.js
// chunk 404'd, and next/image requests 400'd. Root cause: live
// deployments are served under /api/v1/apps/<appID>/live/, not domain
// root, but Next.js's static export always emits root-absolute asset
// paths (/_next/static/...) unless told the real basePath — something
// the LLM has no way to know at generation time (it doesn't know its own
// future appID or how serving is structured), so this can never be fixed
// by better prompting alone, unlike the tsconfig.json gap above. Also:
// next/image's default loader is flatly incompatible with `output:
// 'export'` without images.unoptimized — Next.js hard-fails the BUILD
// over this, not just a runtime 400, if the app uses next/image at all.
func TestPublishBundleActivity_ForcesNextConfigBasePath(t *testing.T) {
	fp := &fakeProvisioner{}
	fb := &fakeBuilder{buildFn: func(files map[string]string) (map[string]string, string, error) {
		return files, "", nil
	}}
	setFakes(t, fp, fb)

	appID := uuid.New()
	_, err := agents.PublishBundleActivity(context.Background(), apptemporal.PublishBundleInput{
		RunID: uuid.New(),
		AppID: appID,
		Files: map[string]string{
			"next.config.js":   "module.exports = { output: 'export' }",
			"src/app/page.tsx": "export default function Home() { return null }",
		},
	})
	require.NoError(t, err)
	require.Len(t, fb.calls, 1)
	gotNextConfig := fb.calls[0]["next.config.js"]
	wantBasePath := "/api/v1/apps/" + appID.String() + "/live"
	assert.Contains(t, gotNextConfig, wantBasePath, "next.config.js must be force-corrected with this app's actual live basePath")
	assert.Contains(t, gotNextConfig, "images", "must set images.unoptimized — next/image's default loader is incompatible with output:'export' and fails the BUILD, not just a runtime request")
	assert.Contains(t, gotNextConfig, "unoptimized")
}

// TestPublishBundleActivity_ForcesGlobalsCSSImport is a regression test for
// a real production failure that reached a genuinely successful deploy
// with correctly-resolving JS chunks and everything: layout.tsx never
// imported globals.css, so the app shipped with ZERO CSS — no <link
// rel="stylesheet"> at all, confirmed directly on the live page (verified
// via the built-in browser: document.querySelectorAll('link,style')
// returned nothing but a script preload). Not a build failure — Next.js
// happily builds an app that never imports its own stylesheet — so
// nothing upstream (self-heal, QA's build check, the repair loop) could
// ever have caught this; the build itself was perfect and still wrong.
func TestPublishBundleActivity_ForcesGlobalsCSSImport(t *testing.T) {
	fp := &fakeProvisioner{}
	fb := &fakeBuilder{buildFn: func(files map[string]string) (map[string]string, string, error) {
		return files, "", nil
	}}
	setFakes(t, fp, fb)

	brokenLayout := `import Header from "@/components/Header";
export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (<html><body><Header />{children}</body></html>);
}`
	_, err := agents.PublishBundleActivity(context.Background(), apptemporal.PublishBundleInput{
		RunID: uuid.New(),
		AppID: uuid.New(),
		Files: map[string]string{
			"src/app/layout.tsx":  brokenLayout,
			"src/app/globals.css": "@tailwind base;\n@tailwind components;\n@tailwind utilities;",
		},
	})
	require.NoError(t, err)
	require.Len(t, fb.calls, 1)
	gotLayout := fb.calls[0]["src/app/layout.tsx"]
	assert.Contains(t, gotLayout, "globals.css", "layout.tsx must be force-corrected to import globals.css, or the deployed app ships with zero CSS")
	assert.Contains(t, gotLayout, "Header", "the correction must not clobber the rest of the file's real content")
}

// TestPublishSourceActivity_ForcesTsConfigPathAlias is the same regression
// as TestPublishBundleActivity_ForcesTsConfigPathAlias_BeforeBuild, but for
// the SAVED source — so that a later RedeployWorkflow rebuild (which fetches
// this saved source, not the original codegen output) also gets the
// correction, not just this run's own immediate build.
func TestPublishSourceActivity_ForcesTsConfigPathAlias(t *testing.T) {
	fp := &fakeProvisioner{}
	setFakes(t, fp, &fakeBuilder{})

	brokenTsConfig := `{"compilerOptions":{"target":"es5","strict":true}}`
	_, err := agents.PublishSourceActivity(context.Background(), apptemporal.PublishSourceInput{
		RunID: uuid.New(),
		Files: map[string]string{
			"tsconfig.json":    brokenTsConfig,
			"src/app/page.tsx": `import Header from "@/components/Header"`,
		},
	})
	require.NoError(t, err)
	require.Len(t, fp.provisionSourceCalls, 1)
	gotTsConfig := fp.provisionSourceCalls[0]["tsconfig.json"]
	assert.NotEqual(t, brokenTsConfig, gotTsConfig, "saved source's tsconfig.json was never corrected")
	assert.Contains(t, gotTsConfig, `"@/*"`)
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
