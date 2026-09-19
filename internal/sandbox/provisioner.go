package sandbox

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Provisioner interface {
	Provision(ctx context.Context, runID uuid.UUID, files map[string]string) (previewURL string, expiresAt time.Time, err error)
	Promote(ctx context.Context, appID, runID uuid.UUID) (liveURL string, err error)
	Teardown(ctx context.Context, runID uuid.UUID) error
}