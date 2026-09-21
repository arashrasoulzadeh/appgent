package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Client struct {
	mc *minio.Client
}

// NewClient connects to an S3-compatible object store (MinIO). endpoint may
// include a scheme (http://minio:9000) or be a bare host:port — either way
// useSSL decides which scheme the client actually uses.
func NewClient(endpoint, accessKey, secretKey string, useSSL bool) (*Client, error) {
	host := endpoint
	if u, err := url.Parse(endpoint); err == nil && u.Host != "" {
		host = u.Host
		useSSL = u.Scheme == "https"
	}

	mc, err := minio.New(host, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}
	return &Client{mc: mc}, nil
}

func (c *Client) EnsureBucket(ctx context.Context, bucket string) error {
	exists, err := c.mc.BucketExists(ctx, bucket)
	if err != nil {
		return fmt.Errorf("check bucket %s: %w", bucket, err)
	}
	if exists {
		return nil
	}
	if err := c.mc.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("create bucket %s: %w", bucket, err)
	}
	return nil
}

func (c *Client) Upload(ctx context.Context, bucket, objectName string, reader io.Reader, size int64, contentType string) error {
	_, err := c.mc.PutObject(ctx, bucket, objectName, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("upload %s/%s: %w", bucket, objectName, err)
	}
	return nil
}

func (c *Client) Download(ctx context.Context, bucket, objectName string) (io.ReadCloser, error) {
	obj, err := c.mc.GetObject(ctx, bucket, objectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get %s/%s: %w", bucket, objectName, err)
	}
	// GetObject doesn't error until the first read/stat, so surface a
	// missing object here rather than handing the caller a "successful"
	// reader that fails on first use.
	if _, err := obj.Stat(); err != nil {
		obj.Close()
		return nil, fmt.Errorf("stat %s/%s: %w", bucket, objectName, err)
	}
	return obj, nil
}

type ObjectInfo struct {
	Key  string
	Size int64
}

func (c *Client) ListObjects(ctx context.Context, bucket, prefix string) ([]ObjectInfo, error) {
	var objects []ObjectInfo
	for obj := range c.mc.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("list %s/%s: %w", bucket, prefix, obj.Err)
		}
		objects = append(objects, ObjectInfo{Key: obj.Key, Size: obj.Size})
	}
	return objects, nil
}

func (c *Client) CopyObject(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) error {
	_, err := c.mc.CopyObject(ctx,
		minio.CopyDestOptions{Bucket: dstBucket, Object: dstKey},
		minio.CopySrcOptions{Bucket: srcBucket, Object: srcKey},
	)
	if err != nil {
		return fmt.Errorf("copy %s/%s to %s/%s: %w", srcBucket, srcKey, dstBucket, dstKey, err)
	}
	return nil
}

func (c *Client) Delete(ctx context.Context, bucket, objectName string) error {
	if err := c.mc.RemoveObject(ctx, bucket, objectName, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("delete %s/%s: %w", bucket, objectName, err)
	}
	return nil
}

// DeletePrefix removes every object under prefix (e.g. a whole run's
// published bundle).
func (c *Client) DeletePrefix(ctx context.Context, bucket, prefix string) error {
	objCh := c.mc.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true})
	for obj := range objCh {
		if obj.Err != nil {
			return fmt.Errorf("list %s/%s: %w", bucket, prefix, obj.Err)
		}
		if err := c.mc.RemoveObject(ctx, bucket, obj.Key, minio.RemoveObjectOptions{}); err != nil {
			return fmt.Errorf("delete %s/%s: %w", bucket, obj.Key, err)
		}
	}
	return nil
}

func (c *Client) Exists(ctx context.Context, bucket, objectName string) (bool, error) {
	_, err := c.mc.StatObject(ctx, bucket, objectName, minio.StatObjectOptions{})
	if err != nil {
		errResp := minio.ToErrorResponse(err)
		if errResp.Code == "NoSuchKey" || strings.Contains(err.Error(), "does not exist") {
			return false, nil
		}
		return false, fmt.Errorf("stat %s/%s: %w", bucket, objectName, err)
	}
	return true, nil
}
