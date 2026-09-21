package sandbox

import (
	"context"
	"fmt"
	"io"
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

// SourcePrefix is where a run's raw generated source (pre-build) is kept —
// a separate top-level namespace from RunPrefix (which holds the BUILT
// output), not nested under it: Promote's ListObjects(RunPrefix(runID))
// would otherwise also match anything under a "runs/<id>/source/" nested
// prefix and copy raw .tsx source into the live site alongside the built
// index.html. Source is saved unconditionally whenever generation
// finishes, so a failed build can be retried (RedeployWorkflow) without
// regenerating from scratch.
func SourcePrefix(runID uuid.UUID) string {
	return fmt.Sprintf("sources/%s/", runID.String())
}

func contentTypeFor(path string) string {
	if ct := mime.TypeByExtension(filepath.Ext(path)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// Provision uploads files to this run's prefix in object storage.
func (p *StaticExportProvisioner) Provision(ctx context.Context, runID uuid.UUID, files map[string]string) error {
	return p.uploadFiles(ctx, RunPrefix(runID), files)
}

// ProvisionSource uploads a run's raw generated source, separately from
// the built output Provision publishes.
func (p *StaticExportProvisioner) ProvisionSource(ctx context.Context, runID uuid.UUID, files map[string]string) error {
	return p.uploadFiles(ctx, SourcePrefix(runID), files)
}

func (p *StaticExportProvisioner) uploadFiles(ctx context.Context, prefix string, files map[string]string) error {
	if err := p.storageClient.EnsureBucket(ctx, p.bucket); err != nil {
		return fmt.Errorf("ensure bucket: %w", err)
	}
	for path, content := range files {
		objectName := prefix + path
		if err := p.storageClient.Upload(ctx, p.bucket, objectName, strings.NewReader(content), int64(len(content)), contentTypeFor(path)); err != nil {
			return fmt.Errorf("upload %s: %w", objectName, err)
		}
	}
	return nil
}

// FetchSource downloads a run's previously provisioned raw source back
// into memory, keyed by path relative to the source prefix.
func (p *StaticExportProvisioner) FetchSource(ctx context.Context, runID uuid.UUID) (map[string]string, error) {
	prefix := SourcePrefix(runID)
	objects, err := p.storageClient.ListObjects(ctx, p.bucket, prefix)
	if err != nil {
		return nil, fmt.Errorf("list source objects: %w", err)
	}
	if len(objects) == 0 {
		return nil, fmt.Errorf("run %s has no saved source under %s", runID, prefix)
	}

	files := make(map[string]string, len(objects))
	for _, obj := range objects {
		rc, err := p.storageClient.Download(ctx, p.bucket, obj.Key)
		if err != nil {
			return nil, fmt.Errorf("download %s: %w", obj.Key, err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", obj.Key, err)
		}
		files[obj.Key[len(prefix):]] = string(content)
	}
	return files, nil
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
