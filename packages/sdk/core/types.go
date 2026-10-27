// Package core defines the provider adapter contract.
package core

import (
	"context"
	"io"
	"time"
)

// UploadRequest describes an object to upload.
type UploadRequest struct {
	Source      io.Reader
	Bucket      string
	Key         string
	ContentType string
}

// UploadResult contains the uploaded object's URL.
type UploadResult struct {
	URL string
}

// Object identifies a stored object and its available metadata.
type Object struct {
	Bucket      string
	Key         string
	URL         string
	Size        int64
	ContentType string
	ETag        string
	UpdatedAt   time.Time
}

type GetRequest struct{ Bucket, Key string }
type GetResult struct {
	Object Object
	Body   io.ReadCloser
}
type ListRequest struct {
	Bucket, Prefix, Cursor string
	Limit                  int
}
type ListResult struct {
	Objects    []Object
	NextCursor string
}
type DeleteRequest struct{ Bucket, Key string }
type DeleteBucketRequest struct{ Bucket string }

// BucketAdapter lists the buckets or stores visible to the configured credentials.
type BucketAdapter interface {
	ListBuckets(context.Context) ([]string, error)
}

type BucketDeleter interface {
	DeleteBucket(context.Context, DeleteBucketRequest) error
}

// ObjectAdapter supports reading, listing, and deleting stored objects.
type ObjectAdapter interface {
	Get(context.Context, GetRequest) (GetResult, error)
	List(context.Context, ListRequest) (ListResult, error)
	Delete(context.Context, DeleteRequest) error
}

// Adapter implements uploads for one storage provider.
type Adapter interface {
	Upload(context.Context, UploadRequest) (UploadResult, error)
}
