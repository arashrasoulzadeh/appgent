package sandbox

import (
	"context"

	"github.com/google/uuid"
)

// Provisioner publishes a run's generated files to storage and promotes a
// run's bundle to be an app's live deployment. URL construction is the
// caller's responsibility (see internal/handlers — files are served back
// over HTTP by this app's own API rather than exposing storage URLs
// directly).
type Provisioner interface {
	// ProvisionSource durably saves a run's raw generated source (before
	// build), separately from the built output Provision publishes. This
	// is what lets a build be retried later (via RedeployWorkflow) without
	// needing a full regenerate — called unconditionally whenever a run
	// finishes generating, regardless of whether the build itself
	// succeeds.
	ProvisionSource(ctx context.Context, runID uuid.UUID, files map[string]string) error
	// FetchSource retrieves a run's previously provisioned raw source.
	FetchSource(ctx context.Context, runID uuid.UUID) (map[string]string, error)

	Provision(ctx context.Context, runID uuid.UUID, files map[string]string) error
	Promote(ctx context.Context, appID, runID uuid.UUID) error
	Teardown(ctx context.Context, runID uuid.UUID) error
}
