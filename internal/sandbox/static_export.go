package sandbox

import (
	"context"
	"fmt"
	"mime"
	"path/filepath"
	"strings"

	"github.com/arashrasoulzadeh/appgent/internal/storage"
	"github.com/google/uuid"
)

// StaticExportProvisioner publishes a run's generated files to object
// storage and promotes a run's bundle to an app's stable "live" prefix on
// deploy. It does not itself decide public URLs — those are constructed by
// the API's preview/live handlers, which serve the objects back over HTTP
// rather than exposing the object store directly.
type StaticExportProvisioner struct {
	storageClient *storage.Client
	bucket        string
}

func NewStaticExportProvisioner(storageClient *storage.Client, bucket string) *StaticExportProvisioner {
	return &StaticExportProvisioner{storageClient: storageClient, bucket: bucket}
}

// RunPrefix is the object-storage key prefix a run's files are published
// under.
func RunPrefix(runID uuid.UUID) string {
	return fmt.Sprintf("runs/%s/", runID.String())
}

// AppLivePrefix is the stable object-storage key prefix an app's current
// deployment lives under — overwritten on every deploy, so there's always
// exactly one canonical "live" prefix per app.
func AppLivePrefix(appID uuid.UUID) string {
	return fmt.Sprintf("apps/%s/", appID.String())
}

func contentTypeFor(path string) string {
	if ct := mime.TypeByExtension(filepath.Ext(path)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// Provision uploads files to this run's prefix in object storage.
func (p *StaticExportProvisioner) Provision(ctx context.Context, runID uuid.UUID, files map[string]string) error {
	if err := p.storageClient.EnsureBucket(ctx, p.bucket); err != nil {
		return fmt.Errorf("ensure bucket: %w", err)
	}
	prefix := RunPrefix(runID)
	for path, content := range files {
		objectName := prefix + path
		if err := p.storageClient.Upload(ctx, p.bucket, objectName, strings.NewReader(content), int64(len(content)), contentTypeFor(path)); err != nil {
			return fmt.Errorf("upload %s: %w", objectName, err)
		}
	}
	return nil
}

// Promote copies a run's published bundle to the app's stable live prefix,
// overwriting whatever was there before.
func (p *StaticExportProvisioner) Promote(ctx context.Context, appID, runID uuid.UUID) error {
	srcPrefix := RunPrefix(runID)
	dstPrefix := AppLivePrefix(appID)

	objects, err := p.storageClient.ListObjects(ctx, p.bucket, srcPrefix)
	if err != nil {
		return fmt.Errorf("list source objects: %w", err)
	}
	if len(objects) == 0 {
		return fmt.Errorf("run %s has no published files under %s", runID, srcPrefix)
	}

	for _, obj := range objects {
		dstName := dstPrefix + obj.Key[len(srcPrefix):]
		if err := p.storageClient.CopyObject(ctx, p.bucket, obj.Key, p.bucket, dstName); err != nil {
			return fmt.Errorf("copy %s to %s: %w", obj.Key, dstName, err)
		}
	}
	return nil
}

func (p *StaticExportProvisioner) Teardown(ctx context.Context, runID uuid.UUID) error {
	return p.storageClient.DeletePrefix(ctx, p.bucket, RunPrefix(runID))
}
