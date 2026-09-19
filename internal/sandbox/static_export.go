package sandbox

import (
	"context"
	"fmt"
	"time"

	"github.com/arashrasoulzadeh/appgent/internal/storage"
	"github.com/google/uuid"
)

type StaticExportProvisioner struct {
	storageClient  *storage.Client
	bucket         string
	previewBaseURL string
	liveBaseURL    string
}

func NewStaticExportProvisioner(storageClient *storage.Client, bucket, previewBaseURL, liveBaseURL string) *StaticExportProvisioner {
	return &StaticExportProvisioner{
		storageClient:  storageClient,
		bucket:         bucket,
		previewBaseURL: previewBaseURL,
		liveBaseURL:    liveBaseURL,
	}
}

func (p *StaticExportProvisioner) Provision(ctx context.Context, runID uuid.UUID, files map[string]string) (string, time.Time, error) {
	prefix := fmt.Sprintf("runs/%s/", runID.String())
	for path, content := range files {
		objectName := prefix + path
		_, err := p.storageClient.Upload(ctx, p.bucket, objectName, nil, int64(len(content)))
		if err != nil {
			return "", time.Time{}, fmt.Errorf("upload %s: %w", objectName, err)
		}
	}

	previewURL := fmt.Sprintf("%s/runs/%s/index.html", p.previewBaseURL, runID.String())
	expiresAt := time.Now().Add(1 * time.Hour)

	return previewURL, expiresAt, nil
}

func (p *StaticExportProvisioner) Promote(ctx context.Context, appID, runID uuid.UUID) (string, error) {
	appSlug := appID.String()[:8]

	srcPrefix := fmt.Sprintf("runs/%s/", runID.String())
	dstPrefix := fmt.Sprintf("apps/%s/", appSlug)

	objects, err := p.storageClient.ListObjects(ctx, p.bucket, srcPrefix)
	if err != nil {
		return "", fmt.Errorf("list source objects: %w", err)
	}

	for _, obj := range objects {
		dstName := dstPrefix + obj.Key[len(srcPrefix):]
		_, err := p.storageClient.CopyObject(ctx, p.bucket, obj.Key, p.bucket, dstName)
		if err != nil {
			return "", fmt.Errorf("copy %s to %s: %w", obj.Key, dstName, err)
		}
	}

	liveURL := fmt.Sprintf("https://%s.%s", appSlug, p.liveBaseURL)
	return liveURL, nil
}

func (p *StaticExportProvisioner) Teardown(ctx context.Context, runID uuid.UUID) error {
	return nil
}