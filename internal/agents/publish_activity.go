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

func PublishBundleActivity(ctx context.Context, in temporal.PublishBundleInput) (temporal.PublishBundleOutput, error) {
	if provisioner == nil {
		return temporal.PublishBundleOutput{}, fmt.Errorf("provisioner not configured")
	}
	if builder == nil {
		return temporal.PublishBundleOutput{}, fmt.Errorf("builder not configured")
	}
	if len(in.Files) == 0 {
		return temporal.PublishBundleOutput{}, fmt.Errorf("no files to publish")
	}

	built, err := builder.Build(ctx, in.RunID, in.Files)
	if err != nil {
		return temporal.PublishBundleOutput{}, fmt.Errorf("build: %w", err)
	}

	if err := provisioner.Provision(ctx, in.RunID, built); err != nil {
		return temporal.PublishBundleOutput{}, fmt.Errorf("provision bundle: %w", err)
	}
	return temporal.PublishBundleOutput{BundlePath: sandbox.RunPrefix(in.RunID)}, nil
}
