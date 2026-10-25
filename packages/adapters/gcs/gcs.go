// Package gcs uploads objects to Google Cloud Storage.
package gcs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/akashprasad/file-uploader/packages/sdk/core"
)

const uploadEndpoint = "https://storage.googleapis.com/upload/storage/v1/b/"

// TokenSource returns an OAuth 2.0 access token.
type TokenSource func(context.Context) (string, error)

// Config configures the JSON API client.
type Config struct {
	TokenSource TokenSource
	HTTPClient  *http.Client
}

// Adapter uploads to Google Cloud Storage.
type Adapter struct {
	tokenSource TokenSource
	httpClient  *http.Client
}

// New creates a GCS adapter.
func New(config Config) (*Adapter, error) {
	if config.TokenSource == nil {
		return nil, fmt.Errorf("Google Cloud Storage OAuth token source is required")
	}
	client := config.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &Adapter{tokenSource: config.TokenSource, httpClient: client}, nil
}

// Upload writes an object through the JSON API.
func (a *Adapter) Upload(ctx context.Context, request core.UploadRequest) (core.UploadResult, error) {
	if request.Source == nil {
		return core.UploadResult{}, fmt.Errorf("upload source is required")
	}
	if request.Bucket == "" {
		return core.UploadResult{}, fmt.Errorf("bucket is required")
	}
	if request.Key == "" {
		return core.UploadResult{}, fmt.Errorf("object key is required")
	}

	token, err := a.tokenSource(ctx)
	if err != nil {
		return core.UploadResult{}, fmt.Errorf("get Google Cloud Storage OAuth token: %w", err)
	}
	if strings.TrimSpace(token) == "" {
		return core.UploadResult{}, fmt.Errorf("Google Cloud Storage OAuth token is empty")
	}

	endpoint := uploadEndpoint + url.PathEscape(request.Bucket) + "/o"
	query := url.Values{"uploadType": {"media"}, "name": {request.Key}}
	endpoint += "?" + query.Encode()
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, request.Source)
	if err != nil {
		return core.UploadResult{}, fmt.Errorf("create Google Cloud Storage upload request: %w", err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+token)
	if request.ContentType != "" {
		httpRequest.Header.Set("Content-Type", request.ContentType)
	}

	response, err := a.httpClient.Do(httpRequest)
	if err != nil {
		return core.UploadResult{}, fmt.Errorf("upload object to Google Cloud Storage: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return core.UploadResult{}, fmt.Errorf("Google Cloud Storage upload failed (%s): %s", response.Status, strings.TrimSpace(string(message)))
	}

	objectURL := url.URL{Scheme: "https", Host: "storage.googleapis.com", Path: "/" + request.Bucket + "/" + request.Key}
	return core.UploadResult{URL: objectURL.String()}, nil
}

var _ core.Adapter = (*Adapter)(nil)
