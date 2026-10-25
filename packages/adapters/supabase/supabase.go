// Package supabase uploads objects to Supabase Storage.
package supabase

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/akashprasad/file-uploader/packages/sdk/core"
)

// TokenSource returns a Supabase access token.
type TokenSource func(context.Context) (string, error)

// Config configures the Supabase Storage API client.
type Config struct {
	ProjectURL  string
	APIKey      string
	TokenSource TokenSource
	Upsert      bool
	Public      bool
	HTTPClient  *http.Client
}

// Adapter uploads objects to Supabase Storage.
type Adapter struct {
	projectURL *url.URL
	apiKey     string
	token      TokenSource
	upsert     bool
	public     bool
	client     *http.Client
}

// New creates a Supabase Storage adapter.
func New(config Config) (*Adapter, error) {
	projectURL, err := url.Parse(config.ProjectURL)
	if err != nil || projectURL.Scheme != "https" || projectURL.Host == "" {
		return nil, fmt.Errorf("a valid HTTPS Supabase project URL is required")
	}
	if config.APIKey == "" {
		return nil, fmt.Errorf("Supabase API key is required")
	}
	client := config.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &Adapter{projectURL, config.APIKey, config.TokenSource, config.Upsert, config.Public, client}, nil
}

// Upload writes an object to the configured Supabase project.
func (a *Adapter) Upload(ctx context.Context, request core.UploadRequest) (core.UploadResult, error) {
	if request.Source == nil || request.Bucket == "" || request.Key == "" {
		return core.UploadResult{}, fmt.Errorf("upload source, bucket, and key are required")
	}
	endpoint := *a.projectURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/storage/v1/object/" + request.Bucket + "/" + request.Key
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), request.Source)
	if err != nil {
		return core.UploadResult{}, fmt.Errorf("create Supabase upload request: %w", err)
	}
	httpRequest.Header.Set("apikey", a.apiKey)
	if a.token != nil {
		token, err := a.token(ctx)
		if err != nil {
			return core.UploadResult{}, fmt.Errorf("get Supabase access token: %w", err)
		}
		if strings.TrimSpace(token) == "" {
			return core.UploadResult{}, fmt.Errorf("Supabase access token is empty")
		}
		httpRequest.Header.Set("Authorization", "Bearer "+token)
	}
	if request.ContentType != "" {
		httpRequest.Header.Set("Content-Type", request.ContentType)
	}
	if a.upsert {
		httpRequest.Header.Set("x-upsert", "true")
	}

	response, err := a.client.Do(httpRequest)
	if err != nil {
		return core.UploadResult{}, fmt.Errorf("upload object to Supabase Storage: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return core.UploadResult{}, fmt.Errorf("Supabase Storage upload failed (%s): %s", response.Status, strings.TrimSpace(string(body)))
	}

	access := "authenticated"
	if a.public {
		access = "public"
	}
	objectURL := *a.projectURL
	objectURL.Path = strings.TrimRight(objectURL.Path, "/") + "/storage/v1/object/" + access + "/" + request.Bucket + "/" + request.Key
	return core.UploadResult{URL: objectURL.String()}, nil
}

var _ core.Adapter = (*Adapter)(nil)
