// Package gcs uploads objects to Google Cloud Storage.
package gcs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

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

func (a *Adapter) objectRequest(ctx context.Context, method, endpoint string) (*http.Response, error) {
	token, err := a.tokenSource(ctx)
	if err != nil {
		return nil, fmt.Errorf("get Google Cloud Storage OAuth token: %w", err)
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("Google Cloud Storage OAuth token is empty")
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return a.httpClient.Do(req)
}

func (a *Adapter) Get(ctx context.Context, r core.GetRequest) (core.GetResult, error) {
	u := "https://storage.googleapis.com/storage/v1/b/" + url.PathEscape(r.Bucket) + "/o/" + url.PathEscape(r.Key) + "?alt=media"
	resp, err := a.objectRequest(ctx, http.MethodGet, u)
	if err != nil {
		return core.GetResult{}, err
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return core.GetResult{}, fmt.Errorf("Google Cloud Storage get failed (%s): %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return core.GetResult{Object: core.Object{Bucket: r.Bucket, Key: r.Key, URL: "https://storage.googleapis.com/" + r.Bucket + "/" + r.Key, ContentType: resp.Header.Get("Content-Type"), ETag: resp.Header.Get("ETag")}, Body: resp.Body}, nil
}

func (a *Adapter) List(ctx context.Context, r core.ListRequest) (core.ListResult, error) {
	q := url.Values{"prefix": {r.Prefix}, "maxResults": {strconv.Itoa(pageLimit(r.Limit))}}
	if r.Cursor != "" {
		q.Set("pageToken", r.Cursor)
	}
	resp, err := a.objectRequest(ctx, http.MethodGet, "https://storage.googleapis.com/storage/v1/b/"+url.PathEscape(r.Bucket)+"/o?"+q.Encode())
	if err != nil {
		return core.ListResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return core.ListResult{}, fmt.Errorf("Google Cloud Storage list failed (%s): %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var data struct {
		Next  string `json:"nextPageToken"`
		Items []struct {
			Name        string `json:"name"`
			ContentType string `json:"contentType"`
			Size        string `json:"size"`
			ETag        string `json:"etag"`
			Updated     string `json:"updated"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return core.ListResult{}, err
	}
	out := core.ListResult{NextCursor: data.Next}
	for _, o := range data.Items {
		size, _ := strconv.ParseInt(o.Size, 10, 64)
		updated, _ := time.Parse(time.RFC3339, o.Updated)
		out.Objects = append(out.Objects, core.Object{Bucket: r.Bucket, Key: o.Name, URL: "https://storage.googleapis.com/" + r.Bucket + "/" + o.Name, Size: size, ContentType: o.ContentType, ETag: o.ETag, UpdatedAt: updated})
	}
	return out, nil
}

func (a *Adapter) Delete(ctx context.Context, r core.DeleteRequest) error {
	u := "https://storage.googleapis.com/storage/v1/b/" + url.PathEscape(r.Bucket) + "/o/" + url.PathEscape(r.Key)
	resp, err := a.objectRequest(ctx, http.MethodDelete, u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Google Cloud Storage delete failed (%s): %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return nil
}

func pageLimit(n int) int {
	if n <= 0 {
		return 100
	}
	if n > 1000 {
		return 1000
	}
	return n
}

var _ core.ObjectAdapter = (*Adapter)(nil)
