// Package vercel uploads files to Vercel Blob.
package vercel

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/akashprasad/file-uploader/packages/sdk/core"
)

const defaultAPIEndpoint = "https://vercel.com/api/blob"

// Config configures a Vercel Blob client.
type Config struct {
	Token       string
	StoreID     string
	Access      string // "public" or "private"
	APIEndpoint string
	APIVersion  string
	HTTPClient  *http.Client
}

// Adapter uploads to Vercel Blob.
type Adapter struct {
	token      string
	storeID    string
	access     string
	endpoint   string
	apiVersion string
	client     *http.Client
}

// New creates an adapter for a Vercel Blob store.
func New(config Config) (*Adapter, error) {
	if config.Token == "" || config.StoreID == "" {
		return nil, fmt.Errorf("Vercel Blob token and store ID are required")
	}
	if config.Access != "public" && config.Access != "private" {
		return nil, fmt.Errorf("Vercel Blob access must be public or private")
	}
	endpoint := config.APIEndpoint
	if endpoint == "" {
		endpoint = defaultAPIEndpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, fmt.Errorf("a valid HTTPS Vercel Blob API endpoint is required")
	}
	version := config.APIVersion
	if version == "" {
		version = "13"
	}
	client := config.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &Adapter{config.Token, config.StoreID, config.Access, strings.TrimRight(endpoint, "/"), version, client}, nil
}

// Upload stores the object at request.Key and returns the URL from Vercel.
func (a *Adapter) Upload(ctx context.Context, request core.UploadRequest) (core.UploadResult, error) {
	if request.Source == nil || request.Key == "" {
		return core.UploadResult{}, fmt.Errorf("upload source and pathname are required")
	}
	requestURL := a.endpoint + "/?" + url.Values{"pathname": {request.Key}}.Encode()
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPut, requestURL, request.Source)
	if err != nil {
		return core.UploadResult{}, fmt.Errorf("create Vercel Blob upload request: %w", err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+a.token)
	httpRequest.Header.Set("x-vercel-blob-store-id", a.storeID)
	httpRequest.Header.Set("x-api-version", a.apiVersion)
	httpRequest.Header.Set("x-vercel-blob-access", a.access)
	requestID := make([]byte, 16)
	if _, err := rand.Read(requestID); err != nil {
		return core.UploadResult{}, fmt.Errorf("generate Vercel Blob request ID: %w", err)
	}
	httpRequest.Header.Set("x-api-blob-request-id", a.storeID+":"+hex.EncodeToString(requestID))
	httpRequest.Header.Set("x-api-blob-request-attempt", "0")
	if request.ContentType != "" {
		httpRequest.Header.Set("Content-Type", request.ContentType)
	}

	response, err := a.client.Do(httpRequest)
	if err != nil {
		return core.UploadResult{}, fmt.Errorf("upload object to Vercel Blob: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return core.UploadResult{}, fmt.Errorf("Vercel Blob upload failed (%s): %s", response.Status, strings.TrimSpace(string(body)))
	}
	var result struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return core.UploadResult{}, fmt.Errorf("decode Vercel Blob upload response: %w", err)
	}
	if result.URL == "" {
		return core.UploadResult{}, fmt.Errorf("Vercel Blob response did not include an object URL")
	}
	return core.UploadResult{URL: result.URL}, nil
}

var _ core.Adapter = (*Adapter)(nil)
