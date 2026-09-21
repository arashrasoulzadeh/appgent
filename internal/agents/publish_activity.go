package agents

import (
	"context"
	"fmt"

	"github.com/arashrasoulzadeh/appgent/internal/sandbox"
	"github.com/arashrasoulzadeh/appgent/internal/temporal"
)

var provisioner sandbox.Provisioner

// SetProvisioner wires the object-storage-backed provisioner used to
// publish generated bundles. Called once from the worker's main() — see
// services/worker/cmd/worker/main.go.
func SetProvisioner(p sandbox.Provisioner) {
	provisioner = p
}

func PublishBundleActivity(ctx context.Context, in temporal.PublishBundleInput) (temporal.PublishBundleOutput, error) {
	if provisioner == nil {
		return temporal.PublishBundleOutput{}, fmt.Errorf("provisioner not configured")
	}
	if len(in.Files) == 0 {
		return temporal.PublishBundleOutput{}, fmt.Errorf("no files to publish")
	}
	if err := provisioner.Provision(ctx, in.RunID, in.Files); err != nil {
		return temporal.PublishBundleOutput{}, fmt.Errorf("provision bundle: %w", err)
	}
	return temporal.PublishBundleOutput{BundlePath: sandbox.RunPrefix(in.RunID)}, nil
}
