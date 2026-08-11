package s3

import (
	"bytes"
	"context"
	"fmt"

	"github.com/gsoultan/cronos/internal/app/burst"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Config is where objects go.
type Config struct {
	// Endpoint is host:port. Empty means AWS.
	Endpoint  string
	Region    string
	AccessKey string
	SecretKey string
	// Insecure allows plain HTTP. For MinIO on a laptop, and named so nobody
	// sets it without meaning to.
	Insecure bool
}

// putter is what the channel needs from a client, so a test can assert on what
// would have been written rather than on a mock's arguments.
type putter interface {
	PutObject(ctx context.Context, bucket, key string, r *bytes.Reader, size int64,
		opts minio.PutObjectOptions) (minio.UploadInfo, error)
}

// Channel writes documents to object storage.
type Channel struct {
	put putter
}

// New returns a Channel, or an error if it could never write anything.
func New(cfg Config) (*Channel, error) {
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = "s3.amazonaws.com"
	}
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		// Checked at startup: credentials that are missing fail every object
		// of a burst, one at a time, five thousand times.
		return nil, fmt.Errorf("s3: access key and secret key are required")
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: !cfg.Insecure,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("s3: %w", err)
	}
	return &Channel{put: adapt{client}}, nil
}

// Name is what a schedule's `via` matches.
func (c *Channel) Name() string { return "s3" }

// Deliver writes one document.
func (c *Channel) Deliver(ctx context.Context, d burst.Delivery) error {
	dest, err := parse(d.To)
	if err != nil {
		return err
	}

	kind := d.Options["contentType"]
	if kind == "" {
		kind = "application/pdf"
	}

	_, err = c.put.PutObject(ctx, dest.Bucket, dest.Key,
		bytes.NewReader(d.Document), int64(len(d.Document)),
		minio.PutObjectOptions{
			ContentType: kind,
			// The filename a browser offers when somebody downloads it later.
			// Without this an archived statement saves as the key's last
			// segment, which is an id rather than a name.
			ContentDisposition: fmt.Sprintf("attachment; filename=%q", d.Filename),
			UserMetadata: map[string]string{
				// So an object can be traced back to what produced it without
				// consulting a run log that may have rotated.
				"cronos-subject": d.Subject,
			},
		})
	if err != nil {
		return fmt.Errorf("s3: writing %s/%s: %w", dest.Bucket, dest.Key, err)
	}
	return nil
}

// adapt narrows minio's client to the one method this needs.
type adapt struct{ *minio.Client }

func (a adapt) PutObject(ctx context.Context, bucket, key string, r *bytes.Reader,
	size int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	return a.Client.PutObject(ctx, bucket, key, r, size, opts)
}
