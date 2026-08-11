package s3

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/app/burst"
	"github.com/minio/minio-go/v7"
)

type written struct {
	bucket, key string
	body        []byte
	opts        minio.PutObjectOptions
	err         error
}

func (w *written) PutObject(_ context.Context, bucket, key string, r *bytes.Reader,
	_ int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	body, _ := io.ReadAll(r)
	w.bucket, w.key, w.body, w.opts = bucket, key, body, opts
	return minio.UploadInfo{}, w.err
}

func channel() (*Channel, *written) {
	w := &written{}
	return &Channel{put: w}, w
}

func delivery(to string) burst.Delivery {
	return burst.Delivery{
		To: to, Subject: "Your July 2026 statement",
		Filename: "statement-c-1.pdf", Document: []byte("%PDF-1.7 pretend"),
	}
}

func TestItWritesWhereTheUrlSays(t *testing.T) {
	c, w := channel()
	err := c.Deliver(context.Background(),
		delivery("s3://acme-statements/c-1/2026-07.pdf"))
	if err != nil {
		t.Fatal(err)
	}

	if w.bucket != "acme-statements" || w.key != "c-1/2026-07.pdf" {
		t.Errorf("wrote to %s/%s", w.bucket, w.key)
	}
	if string(w.body) != "%PDF-1.7 pretend" {
		t.Errorf("body = %q", w.body)
	}
	if w.opts.ContentType != "application/pdf" {
		t.Errorf("content type = %q", w.opts.ContentType)
	}
	// Without it an archived statement downloads as the key's last segment,
	// which is an id rather than a name.
	if !strings.Contains(w.opts.ContentDisposition, "statement-c-1.pdf") {
		t.Errorf("disposition = %q", w.opts.ContentDisposition)
	}
}

func TestDestinationsAreRefused(t *testing.T) {
	cases := map[string]string{
		"not a url at all": "acme-statements/c-1.pdf",
		"the wrong scheme": "https://acme-statements/c-1.pdf",
		"no bucket":        "s3:///c-1.pdf",
		// Every recipient's statement would land on the same object, so a
		// burst of five thousand leaves one.
		"a bucket with no key":   "s3://acme-statements",
		"a bucket with only a /": "s3://acme-statements/",
		// Keys come from a customer row, and .. confuses every tool that
		// mirrors a bucket onto a filesystem.
		"a key that climbs": "s3://acme-statements/../../etc/passwd",
	}
	for name, to := range cases {
		t.Run(name, func(t *testing.T) {
			c, w := channel()
			if err := c.Deliver(context.Background(), delivery(to)); err == nil {
				t.Error("accepted")
			}
			if w.body != nil {
				t.Error("it wrote anyway")
			}
		})
	}
}

// Credentials that are missing fail every object of a burst, one at a time.
func TestMissingCredentialsFailAtStartup(t *testing.T) {
	if _, err := New(Config{Endpoint: "s3.example:9000"}); err == nil {
		t.Error("a client with no credentials was accepted")
	}
	if _, err := New(Config{AccessKey: "a", SecretKey: "b"}); err != nil {
		t.Errorf("AWS by default should need no endpoint: %v", err)
	}
}

func TestTheContentTypeCanBeOverridden(t *testing.T) {
	c, w := channel()
	d := delivery("s3://b/k.csv")
	d.Options = map[string]string{"contentType": "text/csv"}
	if err := c.Deliver(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if w.opts.ContentType != "text/csv" {
		t.Errorf("content type = %q", w.opts.ContentType)
	}
}
