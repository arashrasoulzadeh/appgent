package agents

import (
	"context"
	"fmt"

	"github.com/arashrasoulzadeh/appgent/internal/sandbox"
	"github.com/arashrasoulzadeh/appgent/internal/temporal"
)

var (
	provisioner sandbox.Provisioner
	builder     sandbox.BuildRunner
)

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
