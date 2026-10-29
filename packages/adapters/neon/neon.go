// Package neon accesses Neon Object Storage over its S3-compatible API.
package neon

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/akashprasad/file-uploader/packages/sdk/core"
)

// Config configures Neon Object Storage for a branch.
type Config struct {
	Endpoint        string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	HTTPClient      *http.Client
	Now             func() time.Time
}

// Adapter accesses Neon Object Storage over S3.
type Adapter struct {
	endpoint  *url.URL
	region    string
	accessKey string
	secretKey string
	client    *http.Client
	now       func() time.Time
}

// New creates a Neon Object Storage adapter.
func New(config Config) (*Adapter, error) {
	endpoint, err := url.Parse(config.Endpoint)
	if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" {
		return nil, fmt.Errorf("a valid HTTPS S3 endpoint is required")
	}
	if config.Region == "" || config.AccessKeyID == "" || config.SecretAccessKey == "" {
		return nil, fmt.Errorf("region and S3 credentials are required")
	}
	client := config.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &Adapter{endpoint: endpoint, region: config.Region, accessKey: config.AccessKeyID, secretKey: config.SecretAccessKey, client: client, now: now}, nil
}

// Upload writes the object using AWS Signature Version 4.
func (a *Adapter) Upload(ctx context.Context, request core.UploadRequest) (core.UploadResult, error) {
	if request.Source == nil || request.Key == "" {
		return core.UploadResult{}, fmt.Errorf("upload source and object key are required")
	}
	if request.Bucket == "" {
		return core.UploadResult{}, fmt.Errorf("bucket is required")
	}

	u := *a.endpoint
	u.Path = strings.TrimRight(u.Path, "/") + "/" + request.Bucket + "/" + request.Key
	stamp := a.now().UTC()
	date, amzDate := stamp.Format("20060102"), stamp.Format("20060102T150405Z")
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u.String(), request.Source)
	if err != nil {
		return core.UploadResult{}, fmt.Errorf("create Neon upload request: %w", err)
	}
	req.Header.Set("x-amz-content-sha256", "UNSIGNED-PAYLOAD")
	req.Header.Set("x-amz-date", amzDate)
	if request.ContentType != "" {
		req.Header.Set("Content-Type", request.ContentType)
	}
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalHeaders := "host:" + u.Host + "\n" +
		"x-amz-content-sha256:UNSIGNED-PAYLOAD\n" + "x-amz-date:" + amzDate + "\n"
	if request.ContentType != "" {
		signedHeaders = "content-type;" + signedHeaders
		canonicalHeaders = "content-type:" + request.ContentType + "\n" + canonicalHeaders
	}
	canonicalURI := u.EscapedPath()
	canonicalRequest := strings.Join([]string{http.MethodPut, canonicalURI, "", canonicalHeaders, signedHeaders, "UNSIGNED-PAYLOAD"}, "\n")
	hash := sha256.Sum256([]byte(canonicalRequest))
	scope := date + "/" + a.region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + fmt.Sprintf("%x", hash)
	signature := sign(a.secretKey, date, a.region, stringToSign)
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+a.accessKey+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+signature)

	response, err := a.client.Do(req)
	if err != nil {
		return core.UploadResult{}, fmt.Errorf("upload object to Neon Object Storage: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return core.UploadResult{}, fmt.Errorf("S3-compatible upload failed (%s): %s", response.Status, strings.TrimSpace(string(body)))
	}
	objectURL := u
	return core.UploadResult{URL: objectURL.String()}, nil
}

func sign(secret, date, region, stringToSign string) string {
	hmacSHA := func(key []byte, value string) []byte {
		mac := hmac.New(sha256.New, key)
		_, _ = mac.Write([]byte(value))
		return mac.Sum(nil)
	}
	kDate := hmacSHA([]byte("AWS4"+secret), date)
	kRegion := hmacSHA(kDate, region)
	kService := hmacSHA(kRegion, "s3")
	kSigning := hmacSHA(kService, "aws4_request")
	sum := hmacSHA(kSigning, stringToSign)
	return fmt.Sprintf("%x", sum)
}

var _ core.Adapter = (*Adapter)(nil)

func (a *Adapter) signedRequest(ctx context.Context, method, bucket, key string, query url.Values) (*http.Response, error) {

	u := *a.endpoint
	u.Path = strings.TrimRight(u.Path, "/")
	if bucket != "" {
		u.Path += "/" + bucket
	}
	if key != "" {
		u.Path += "/" + key
	}
	u.RawQuery = query.Encode()
	stamp := a.now().UTC()
	date, amzDate := stamp.Format("20060102"), stamp.Format("20060102T150405Z")
	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-amz-content-sha256", "UNSIGNED-PAYLOAD")
	req.Header.Set("x-amz-date", amzDate)
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalHeaders := "host:" + u.Host + "\n" + "x-amz-content-sha256:UNSIGNED-PAYLOAD\n" + "x-amz-date:" + amzDate + "\n"
	canonicalQuery := strings.ReplaceAll(u.Query().Encode(), "+", "%20")
	canonicalRequest := strings.Join([]string{method, u.EscapedPath(), canonicalQuery, canonicalHeaders, signedHeaders, "UNSIGNED-PAYLOAD"}, "\n")
	hash := sha256.Sum256([]byte(canonicalRequest))
	scope := date + "/" + a.region + "/s3/aws4_request"
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + fmt.Sprintf("%x", hash)
	signature := sign(a.secretKey, date, a.region, toSign)
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+a.accessKey+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+signature)
	return a.client.Do(req)
}

func (a *Adapter) Get(ctx context.Context, r core.GetRequest) (core.GetResult, error) {
	if r.Bucket == "" || r.Key == "" {
		return core.GetResult{}, fmt.Errorf("bucket and object key are required")
	}
	resp, err := a.signedRequest(ctx, http.MethodGet, r.Bucket, r.Key, nil)
	if err != nil {
		return core.GetResult{}, err
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return core.GetResult{}, fmt.Errorf("S3-compatible get failed (%s): %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return core.GetResult{Object: core.Object{Bucket: r.Bucket, Key: r.Key, ContentType: resp.Header.Get("Content-Type"), ETag: resp.Header.Get("ETag")}, Body: resp.Body}, nil
}

func (a *Adapter) List(ctx context.Context, r core.ListRequest) (core.ListResult, error) {
	if r.Bucket == "" {
		return core.ListResult{}, fmt.Errorf("bucket is required")
	}
	q := url.Values{"list-type": {"2"}, "prefix": {r.Prefix}, "max-keys": {fmt.Sprint(pageLimit(r.Limit))}}
	if r.Cursor != "" {
		q.Set("continuation-token", r.Cursor)
	}
	resp, err := a.signedRequest(ctx, http.MethodGet, r.Bucket, "", q)
	if err != nil {
		return core.ListResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return core.ListResult{}, fmt.Errorf("S3-compatible list failed (%s): %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var page struct {
		Truncated bool   `xml:"IsTruncated"`
		Next      string `xml:"NextContinuationToken"`
		Contents  []struct {
			Key          string    `xml:"Key"`
			Size         int64     `xml:"Size"`
			ETag         string    `xml:"ETag"`
			LastModified time.Time `xml:"LastModified"`
		} `xml:"Contents"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&page); err != nil {
		return core.ListResult{}, err
	}
	out := core.ListResult{}
	for _, obj := range page.Contents {
		out.Objects = append(out.Objects, core.Object{Bucket: r.Bucket, Key: obj.Key, Size: obj.Size, ETag: strings.Trim(obj.ETag, "\""), UpdatedAt: obj.LastModified})
	}
	if page.Truncated {
		out.NextCursor = page.Next
	}
	return out, nil
}

func (a *Adapter) Delete(ctx context.Context, r core.DeleteRequest) error {
	if r.Bucket == "" || r.Key == "" {
		return fmt.Errorf("bucket and object key are required")
	}
	resp, err := a.signedRequest(ctx, http.MethodDelete, r.Bucket, r.Key, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("S3-compatible delete failed (%s): %s", resp.Status, strings.TrimSpace(string(b)))
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
var _ core.BucketAdapter = (*Adapter)(nil)
var _ core.BucketDeleter = (*Adapter)(nil)

func (a *Adapter) ListBuckets(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resp, err := a.signedRequest(ctx, http.MethodGet, "", "", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("S3-compatible bucket list failed (%s): %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var result struct {
		Buckets []struct {
			Name string `xml:"Name"`
		} `xml:"Buckets>Bucket"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	buckets := make([]string, 0, len(result.Buckets))
	for _, b := range result.Buckets {
		buckets = append(buckets, b.Name)
	}
	return buckets, nil
}

func (a *Adapter) DeleteBucket(ctx context.Context, r core.DeleteBucketRequest) error {
	if r.Bucket == "" {
		return fmt.Errorf("bucket is required")
	}
	resp, err := a.signedRequest(ctx, http.MethodDelete, r.Bucket, "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("S3-compatible bucket delete failed (%s): %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return nil
}
