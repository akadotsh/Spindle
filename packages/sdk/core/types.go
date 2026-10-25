// Package core defines the provider adapter contract.
package core

import (
	"context"
	"io"
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

// Adapter implements uploads for one storage provider.
type Adapter interface {
	Upload(context.Context, UploadRequest) (UploadResult, error)
}
