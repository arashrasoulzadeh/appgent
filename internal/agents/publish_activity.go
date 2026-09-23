package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/arashrasoulzadeh/appgent/internal/sandbox"
	"github.com/arashrasoulzadeh/appgent/internal/temporal"
)

var (
	provisioner sandbox.Provisioner
	builder     sandbox.BuildRunner
)

// canonicalTsConfig is the tsconfig.json content code.tmpl instructs the
// LLM to emit verbatim for every generated app — critically, its
// baseUrl/paths mapping is what makes the "@/*" import alias (used by
// EVERY generated file: pages import "@/components/Header", etc.) resolve
// at all under Next.js's webpack build.
const canonicalTsConfig = `{
  "compilerOptions": {
    "target": "es5",
    "lib": ["dom", "dom.iterable", "esnext"],
    "allowJs": true,
    "skipLibCheck": true,
    "strict": true,
    "forceConsistentCasingInFileNames": true,
    "noEmit": true,
    "esModuleInterop": true,
    "module": "esnext",
    "moduleResolution": "bundler",
    "resolveJsonModule": true,
    "isolatedModules": true,
    "jsx": "preserve",
    "incremental": true,
    "baseUrl": ".",
    "paths": { "@/*": ["src/*"] }
  },
  "include": ["next-env.d.ts", "**/*.ts", "**/*.tsx"],
  "exclude": ["node_modules"]
}
`

// ensureTsConfigPathAlias is a deterministic, zero-LLM-token guarantee
// that a build can always resolve "@/..." imports, regardless of whether
// the LLM actually followed code.tmpl's instruction to emit tsconfig.json
// with the required baseUrl/paths mapping. Real production failure mode
// this fixes: the LLM omits (or gets wrong) tsconfig.json's path alias,
// and EVERY "@/components/X" import across the whole app then fails to
// build with "Module not found: Can't resolve '@/components/X'" — which
// looks EXACTLY like a missing-component failure (the error message is
// identical) but isn't: the component files genuinely exist, confirmed by
// inspecting the actual Files map sent to a real build in production. No
// amount of missing-component self-heal or per-file repair can fix this,
// because the files it's "fixing" were never the actual problem — only
// forcing tsconfig.json itself to be correct does. Mutates files in
// place; called right before every real build (QA's build-check,
// PublishBundleActivity, which covers both generation's publish and
// RedeployWorkflow's rebuild).
func ensureTsConfigPathAlias(files map[string]string) {
	current, ok := files["tsconfig.json"]
	if !ok || !strings.Contains(current, `"@/*"`) {
		files["tsconfig.json"] = canonicalTsConfig
	}
}

// nextConfigTemplate is the next.config.js content every generated app
// gets, unconditionally — %s is the basePath. Two things the LLM has no
// way to reliably get right even in principle (not just "sometimes
// forgets", like tsconfig.json):
//
//   - basePath/assetPrefix: live deployments are served under
//     "/api/v1/apps/<appID>/live", never domain root — but the LLM
//     generates code with no idea what appID it's for or how deployment
//     serving is structured, so it can't possibly emit the right value.
//     Without it, EVERY "/_next/static/..." asset the static export
//     references resolves against the wrong (root) path and 404s in
//     production — confirmed live: chunk after chunk 404ing on a
//     genuinely successful build/deploy.
//   - images.unoptimized: `next/image` normally optimizes images through
//     a server-side API route (`/_next/image`) — which doesn't exist for
//     a static export (`output: 'export'`, no server at all). Without
//     this, any `next/image` usage 400s in production (confirmed live:
//     "GET /_next/image?... 400 (Bad Request)"), even though the build
//     itself succeeds (the failure only ever surfaces at runtime, in the
//     browser, not at build time).
const nextConfigTemplate = `/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'export',
  basePath: %q,
  assetPrefix: %q,
  trailingSlash: true,
  images: { unoptimized: true },
}

module.exports = nextConfig
`

// ensureNextConfigBasePath deterministically (zero LLM tokens) force-sets
// next.config.js to the correct basePath/assetPrefix/images config for
// where this app is actually served, overwriting whatever the LLM wrote —
// unlike ensureTsConfigPathAlias, this ALWAYS overwrites rather than only
// filling a gap, because the correct value is deployment-specific and
// never something the LLM could derive on its own in the first place, not
// merely something it sometimes forgets. Mutates files in place; called
// right before every real build, same call sites as
// ensureTsConfigPathAlias.
func ensureNextConfigBasePath(files map[string]string, basePath string) {
	files["next.config.js"] = fmt.Sprintf(nextConfigTemplate, basePath, basePath)
}

// SetProvisioner wires the object-storage-backed provisioner used to
// publish generated bundles. Called once from the worker's main() — see
// services/worker/cmd/worker/main.go.
func SetProvisioner(p sandbox.Provisioner) {
	provisioner = p
}

// SetBuilder wires the builder that compiles a run's generated Next.js
// source into static HTML/CSS/JS before it's published — without this,
// PublishBundleActivity would upload raw .tsx/.ts source files, which
// aren't servable as a website. Called once from the worker's main().
func SetBuilder(b sandbox.BuildRunner) {
	builder = b
}

// PublishSourceActivity durably saves a run's raw generated source (before
// build), independent of PublishBundleActivity's build+publish — called
// first, so source survives even when the build itself fails, letting
// Deploy retry the build later (RedeployWorkflow) without a full
// regenerate. Not tracked as its own agent_steps row (it's a fast, purely
// internal step) — failures are logged by the caller and left non-fatal to
// the run's own status, same as PublishBundleActivity.
func PublishSourceActivity(ctx context.Context, in temporal.PublishSourceInput) (temporal.PublishSourceOutput, error) {
	if provisioner == nil {
		return temporal.PublishSourceOutput{}, fmt.Errorf("provisioner not configured")
	}
	if len(in.Files) == 0 {
		return temporal.PublishSourceOutput{}, fmt.Errorf("no files to publish")
	}
	ensureTsConfigPathAlias(in.Files)
	if err := provisioner.ProvisionSource(ctx, in.RunID, in.Files); err != nil {
		return temporal.PublishSourceOutput{}, fmt.Errorf("provision source: %w", err)
	}
	return temporal.PublishSourceOutput{SourcePath: sandbox.SourcePrefix(in.RunID)}, nil
}

// FetchRunSourceActivity retrieves a run's previously saved raw source,
// plus its original Plan spec and app kind, for RedeployWorkflow to rebuild
// from without asking the LLM to regenerate anything up front — and, if the
// rebuild's real build fails, to repair via a correctly-scoped CodeActivity
// call using that same spec. Spec is the zero value for runs generated
// before the spec column existed; RedeployWorkflow treats that as "no
// repair possible" and falls back to its pre-existing behavior.
func FetchRunSourceActivity(ctx context.Context, in temporal.FetchRunSourceInput) (temporal.FetchRunSourceOutput, error) {
	if provisioner == nil {
		return temporal.FetchRunSourceOutput{}, fmt.Errorf("provisioner not configured")
	}
	files, err := provisioner.FetchSource(ctx, in.RunID)
	if err != nil {
		return temporal.FetchRunSourceOutput{}, fmt.Errorf("fetch source: %w", err)
	}

	out := temporal.FetchRunSourceOutput{Files: files}
	if dbPool == nil {
		return out, nil
	}
	var specJSON []byte
	var appKind string
	err = dbPool.QueryRow(ctx, `
		SELECT gr.spec, a.kind
		FROM generation_runs gr
		JOIN apps a ON a.id = gr.app_id
		WHERE gr.id = $1
	`, in.RunID).Scan(&specJSON, &appKind)
	if err != nil {
		// Non-fatal: RedeployWorkflow just won't be able to repair a build
		// failure for this run, same as if the spec column were unset.
		return out, nil
	}
	out.AppKind = appKind
	if len(specJSON) > 0 {
		_ = json.Unmarshal(specJSON, &out.Spec)
	}
	return out, nil
}

// publishStepOutput is what gets stored in the "publish" agent_steps row's
// output column — the build container's log is the main thing worth
// showing here, since that's the only real diagnostic for a build failure
// (npm/next errors from whatever the LLM generated).
type publishStepOutput struct {
	BundlePath string `json:"bundle_path,omitempty"`
	BuildLog   string `json:"build_log,omitempty"`
}

func PublishBundleActivity(ctx context.Context, in temporal.PublishBundleInput) (temporal.PublishBundleOutput, error) {
	// Tracked as its own "publish" agent_steps row, same as plan/design/
	// code/qa, so the run-detail UI shows this step running (and its build
	// log) instead of the run just appearing to hang between QA finishing
	// and the run's final status landing — this step alone can take
	// several minutes (npm install + next build).
	stepID := startStep(ctx, in.RunID, "publish", 1, "docker-build", in)

	if provisioner == nil {
		err := fmt.Errorf("provisioner not configured")
		finishStep(ctx, stepID, nil, err)
		return temporal.PublishBundleOutput{}, err
	}
	if builder == nil {
		err := fmt.Errorf("builder not configured")
		finishStep(ctx, stepID, nil, err)
		return temporal.PublishBundleOutput{}, err
	}
	if len(in.Files) == 0 {
		err := fmt.Errorf("no files to publish")
		finishStep(ctx, stepID, nil, err)
		return temporal.PublishBundleOutput{}, err
	}
	ensureTsConfigPathAlias(in.Files)
	ensureNextConfigBasePath(in.Files, "/api/v1/apps/"+in.AppID.String()+"/live")

	built, buildLog, err := builder.Build(ctx, in.RunID, in.Files)
	if err != nil {
		buildErr := fmt.Errorf("build: %w", err)
		finishStep(ctx, stepID, publishStepOutput{BuildLog: buildLog}, buildErr)
		return temporal.PublishBundleOutput{}, buildErr
	}

	if err := provisioner.Provision(ctx, in.RunID, built); err != nil {
		provisionErr := fmt.Errorf("provision bundle: %w", err)
		finishStep(ctx, stepID, publishStepOutput{BuildLog: buildLog}, provisionErr)
		return temporal.PublishBundleOutput{}, provisionErr
	}

	bundlePath := sandbox.RunPrefix(in.RunID)
	finishStep(ctx, stepID, publishStepOutput{BundlePath: bundlePath, BuildLog: buildLog}, nil)
	return temporal.PublishBundleOutput{BundlePath: bundlePath}, nil
}
