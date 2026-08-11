// Package s3 archives rendered documents to object storage.
//
// minio-go rather than the AWS SDK: the S3 API is what everything speaks —
// AWS, R2, MinIO, Ceph, Backblaze — and a client that talks the protocol
// rather than one vendor's SDK is the difference between an endpoint setting
// and a rewrite.
//
// # Why archive at all
//
// A statement emailed is a statement somebody can delete. The reproducibility
// story in docs/product.md needs the artifact to still exist next year, which
// makes the object store the record and the mailbox a convenience.
package s3
