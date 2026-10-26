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
	"time"

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

func (a *Adapter) apiRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, a.endpoint+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+a.token)
	req.Header.Set("x-vercel-blob-store-id", a.storeID)
	req.Header.Set("x-api-version", a.apiVersion)
	return a.client.Do(req)
}

func (a *Adapter) Get(ctx context.Context, r core.GetRequest) (core.GetResult, error) {
	host := a.storeID + "." + a.access + ".blob.vercel-storage.com"
	u := url.URL{Scheme: "https", Host: host, Path: "/" + r.Key}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return core.GetResult{}, err
	}
	if a.access == "private" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return core.GetResult{}, err
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return core.GetResult{}, fmt.Errorf("Vercel Blob get failed (%s): %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return core.GetResult{Object: core.Object{Bucket: r.Bucket, Key: r.Key, URL: u.String(), ContentType: resp.Header.Get("Content-Type"), ETag: resp.Header.Get("ETag")}, Body: resp.Body}, nil
}

func (a *Adapter) List(ctx context.Context, r core.ListRequest) (core.ListResult, error) {
	q := url.Values{}
	q.Set("limit", fmt.Sprint(vercelPageLimit(r.Limit)))
	if r.Prefix != "" {
		q.Set("prefix", r.Prefix)
	}
	if r.Cursor != "" {
		q.Set("cursor", r.Cursor)
	}
	resp, err := a.apiRequest(ctx, http.MethodGet, "/?"+q.Encode(), nil)
	if err != nil {
		return core.ListResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return core.ListResult{}, fmt.Errorf("Vercel Blob list failed (%s): %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var data struct {
		Blobs []struct {
			URL, Pathname string
			Size          int64
			UploadedAt    time.Time
			ETag          string
		} `json:"blobs"`
		Cursor  string `json:"cursor"`
		HasMore bool   `json:"hasMore"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return core.ListResult{}, err
	}
	out := core.ListResult{}
	for _, b := range data.Blobs {
		out.Objects = append(out.Objects, core.Object{Bucket: r.Bucket, Key: b.Pathname, URL: b.URL, Size: b.Size, ETag: b.ETag, UpdatedAt: b.UploadedAt})
	}
	if data.HasMore {
		out.NextCursor = data.Cursor
	}
	return out, nil
}

func (a *Adapter) Delete(ctx context.Context, r core.DeleteRequest) error {
	body, _ := json.Marshal(map[string][]string{"urls": {r.Key}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint+"/delete", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.token)
	req.Header.Set("x-vercel-blob-store-id", a.storeID)
	req.Header.Set("x-api-version", a.apiVersion)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Vercel Blob delete failed (%s): %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return nil
}

var _ core.ObjectAdapter = (*Adapter)(nil)
var _ core.BucketAdapter = (*Adapter)(nil)

func (a *Adapter) ListBuckets(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []string{a.storeID}, nil
}

func vercelPageLimit(n int) int {
	if n <= 0 {
		return 100
	}
	if n > 1000 {
		return 1000
	}
	return n
}
