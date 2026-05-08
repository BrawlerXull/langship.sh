// Package logstore archives per-node execution logs to S3-compatible
// object storage (MinIO in dev). The API server writes one object per
// node when the node finishes; the live SSE feed continues to stream
// during execution.
//
// Object key layout:
//   <bucket>/runs/<execID>/<nodeName>.log
package logstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Store is the interface the API server uses. Implementations include the
// MinIO/S3-backed Mongo (mongo) store and a memory store for tests.
type Store interface {
	Put(ctx context.Context, execID, node string, data []byte) error
	Get(ctx context.Context, execID, node string) (io.ReadCloser, error)
	Exists(ctx context.Context, execID, node string) (bool, error)
}

// ErrNotFound is returned by Get when the object doesn't exist.
var ErrNotFound = errors.New("log not found")

// Config carries connection details for a MinIO/S3 endpoint.
type Config struct {
	Endpoint  string // host:port, no scheme — e.g. "minio:9000" or "127.0.0.1:9000"
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

// Minio is the production Store implementation, backed by minio-go.
type Minio struct {
	cli    *minio.Client
	bucket string
}

// NewMinio dials the endpoint with a 10s timeout and ensures the bucket
// exists. Returns an error if either step fails.
func NewMinio(ctx context.Context, cfg Config) (*Minio, error) {
	if cfg.Endpoint == "" {
		return nil, errors.New("MINIO_ENDPOINT is required")
	}
	if cfg.Bucket == "" {
		cfg.Bucket = "flow-logs"
	}
	cli, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}

	bucketCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	exists, err := cli.BucketExists(bucketCtx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("minio bucket check %q: %w", cfg.Bucket, err)
	}
	if !exists {
		if err := cli.MakeBucket(bucketCtx, cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("minio make bucket %q: %w", cfg.Bucket, err)
		}
	}
	return &Minio{cli: cli, bucket: cfg.Bucket}, nil
}

func (m *Minio) Put(ctx context.Context, execID, node string, data []byte) error {
	if execID == "" || node == "" {
		return errors.New("execID and node required")
	}
	key := keyFor(execID, node)
	_, err := m.cli.PutObject(ctx, m.bucket, key,
		bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: "text/plain; charset=utf-8"})
	return err
}

func (m *Minio) Get(ctx context.Context, execID, node string) (io.ReadCloser, error) {
	key := keyFor(execID, node)
	obj, err := m.cli.GetObject(ctx, m.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	// minio-go returns a lazy *Object; calling Stat surfaces NoSuchKey.
	if _, err := obj.Stat(); err != nil {
		_ = obj.Close()
		var er minio.ErrorResponse
		if errors.As(err, &er) && er.Code == "NoSuchKey" {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return obj, nil
}

func (m *Minio) Exists(ctx context.Context, execID, node string) (bool, error) {
	key := keyFor(execID, node)
	_, err := m.cli.StatObject(ctx, m.bucket, key, minio.StatObjectOptions{})
	if err == nil {
		return true, nil
	}
	var er minio.ErrorResponse
	if errors.As(err, &er) && er.Code == "NoSuchKey" {
		return false, nil
	}
	return false, err
}

// keyFor returns the object key for a given execution+node pair. We sanitise
// the node name so weird characters (slashes, spaces) don't bork S3 keys.
func keyFor(execID, node string) string {
	return "runs/" + execID + "/" + sanitize(node) + ".log"
}

func sanitize(s string) string {
	// Replace anything that isn't alnum/-/_ with "-".
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := b.String()
	if out == "" {
		out = "node"
	}
	return out
}
