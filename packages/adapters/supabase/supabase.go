// Package supabase uploads objects to Supabase Storage.
package supabase

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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

func (a *Adapter) authorized(ctx context.Context, method, endpoint string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("apikey", a.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if a.token != nil {
		token, e := a.token(ctx)
		if e != nil {
			return nil, e
		}
		req.Header.Set("Authorization", "Bearer "+token)
	} else {
		req.Header.Set("Authorization", "Bearer "+a.apiKey)
	}
	return a.client.Do(req)
}

func (a *Adapter) Get(ctx context.Context, r core.GetRequest) (core.GetResult, error) {
	endpoint := *a.projectURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/storage/v1/object/" + url.PathEscape(r.Bucket) + "/" + escapeKey(r.Key)
	resp, err := a.authorized(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return core.GetResult{}, err
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return core.GetResult{}, fmt.Errorf("Supabase Storage get failed (%s): %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return core.GetResult{Object: core.Object{Bucket: r.Bucket, Key: r.Key, ContentType: resp.Header.Get("Content-Type"), ETag: resp.Header.Get("ETag")}, Body: resp.Body}, nil
}

func (a *Adapter) List(ctx context.Context, r core.ListRequest) (core.ListResult, error) {
	limit := r.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	offset := 0
	if r.Cursor != "" {
		var err error
		offset, err = strconv.Atoi(r.Cursor)
		if err != nil || offset < 0 {
			return core.ListResult{}, fmt.Errorf("invalid Supabase list cursor")
		}
	}
	body, _ := json.Marshal(map[string]any{"prefix": r.Prefix, "limit": limit, "offset": offset, "sortBy": map[string]string{"column": "name", "order": "asc"}})
	endpoint := *a.projectURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/storage/v1/object/list/" + url.PathEscape(r.Bucket)
	resp, err := a.authorized(ctx, http.MethodPost, endpoint.String(), strings.NewReader(string(body)))
	if err != nil {
		return core.ListResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return core.ListResult{}, fmt.Errorf("Supabase Storage list failed (%s): %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var rows []struct {
		Name     string `json:"name"`
		Metadata struct {
			Size     any    `json:"size"`
			Mimetype string `json:"mimetype"`
		} `json:"metadata"`
		Updated string `json:"updated_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return core.ListResult{}, err
	}
	out := core.ListResult{}
	for _, row := range rows {
		size := int64(0)
		switch v := row.Metadata.Size.(type) {
		case float64:
			size = int64(v)
		case string:
			size, _ = strconv.ParseInt(v, 10, 64)
		}
		out.Objects = append(out.Objects, core.Object{Bucket: r.Bucket, Key: strings.TrimSuffix(r.Prefix, "/") + separator(r.Prefix) + row.Name, Size: size, ContentType: row.Metadata.Mimetype})
	}
	if len(rows) == limit {
		out.NextCursor = strconv.Itoa(offset + len(rows))
	}
	return out, nil
}

func (a *Adapter) Delete(ctx context.Context, r core.DeleteRequest) error {
	body, _ := json.Marshal(map[string][]string{"prefixes": {r.Key}})
	endpoint := *a.projectURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/storage/v1/object/" + url.PathEscape(r.Bucket)
	resp, err := a.authorized(ctx, http.MethodPost, endpoint.String(), strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Supabase Storage delete failed (%s): %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return nil
}

func escapeKey(key string) string {
	parts := strings.Split(key, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}
func separator(prefix string) string {
	if prefix == "" || strings.HasSuffix(prefix, "/") {
		return ""
	}
	return "/"
}

var _ core.ObjectAdapter = (*Adapter)(nil)
var _ core.BucketAdapter = (*Adapter)(nil)

func (a *Adapter) ListBuckets(ctx context.Context) ([]string, error) {
	endpoint := *a.projectURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/storage/v1/bucket"
	resp, err := a.authorized(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Supabase bucket list failed (%s): %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var rows []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, err
	}
	buckets := make([]string, 0, len(rows))
	for _, row := range rows {
		name := row.Name
		if name == "" {
			name = row.ID
		}
		buckets = append(buckets, name)
	}
	return buckets, nil
}
