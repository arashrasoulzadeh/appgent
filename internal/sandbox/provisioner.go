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
	Provision(ctx context.Context, runID uuid.UUID, files map[string]string) error
	Promote(ctx context.Context, appID, runID uuid.UUID) error
	Teardown(ctx context.Context, runID uuid.UUID) error
}
