// Package s3 implements Amazon S3's REST API using Signature Version 4.
package s3

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

type Config struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	PublicBaseURL   string
	HTTPClient      *http.Client
	Now             func() time.Time
}

type Adapter struct {
	endpoint  *url.URL
	region    string
	accessKey string
	secretKey string
	token     string
	publicURL *url.URL
	client    *http.Client
	now       func() time.Time
}

func New(config Config) (*Adapter, error) {
	if config.Region == "" || config.AccessKeyID == "" || config.SecretAccessKey == "" {
		return nil, fmt.Errorf("AWS region, access key ID, and secret access key are required")
	}
	if strings.IndexFunc(config.Region, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-')
	}) >= 0 {
		return nil, fmt.Errorf("AWS region contains invalid characters")
	}
	domain := "amazonaws.com"
	if strings.HasPrefix(config.Region, "cn-") {
		domain = "amazonaws.com.cn"
	}
	endpoint, _ := url.Parse("https://s3." + config.Region + "." + domain)
	var publicURL *url.URL
	if config.PublicBaseURL != "" {
		var err error
		publicURL, err = url.Parse(config.PublicBaseURL)
		if err != nil || publicURL.Scheme != "https" || publicURL.Host == "" || publicURL.User != nil {
			return nil, fmt.Errorf("public base URL must be a valid HTTPS URL")
		}
	}
	client := config.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &Adapter{endpoint: endpoint, region: config.Region, accessKey: config.AccessKeyID, secretKey: config.SecretAccessKey, token: config.SessionToken, publicURL: publicURL, client: client, now: now}, nil
}

func (a *Adapter) request(ctx context.Context, method, bucket, key string, query url.Values, body io.Reader, contentType string) (*http.Response, *url.URL, error) {
	u := *a.endpoint
	u.Path = strings.TrimRight(u.Path, "/")
	if bucket != "" {
		u.Path += "/" + bucket
	}
	if key != "" {
		u.Path += "/" + key
	}
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, nil, err
	}
	stamp := a.now().UTC()
	date, amzDate := stamp.Format("20060102"), stamp.Format("20060102T150405Z")
	req.Header.Set("x-amz-content-sha256", "UNSIGNED-PAYLOAD")
	req.Header.Set("x-amz-date", amzDate)
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalHeaders := "host:" + u.Host + "\n" + "x-amz-content-sha256:UNSIGNED-PAYLOAD\n" + "x-amz-date:" + amzDate + "\n"
	if a.token != "" {
		req.Header.Set("x-amz-security-token", a.token)
		signedHeaders += ";x-amz-security-token"
		canonicalHeaders += "x-amz-security-token:" + a.token + "\n"
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
		signedHeaders = "content-type;" + signedHeaders
		canonicalHeaders = "content-type:" + contentType + "\n" + canonicalHeaders
	}
	canonicalQuery := strings.ReplaceAll(u.Query().Encode(), "+", "%20")
	canonicalRequest := strings.Join([]string{method, u.EscapedPath(), canonicalQuery, canonicalHeaders, signedHeaders, "UNSIGNED-PAYLOAD"}, "\n")
	hash := sha256.Sum256([]byte(canonicalRequest))
	scope := date + "/" + a.region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + fmt.Sprintf("%x", hash)
	dateKey := hmacSHA([]byte("AWS4"+a.secretKey), date)
	regionKey := hmacSHA(dateKey, a.region)
	serviceKey := hmacSHA(regionKey, "s3")
	signingKey := hmacSHA(serviceKey, "aws4_request")
	signature := fmt.Sprintf("%x", hmacSHA(signingKey, stringToSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+a.accessKey+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+signature)
	resp, err := a.client.Do(req)
	return resp, &u, err
}

func hmacSHA(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

func responseError(resp *http.Response, operation string) error {
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("Amazon S3 %s failed (%s): %s", operation, resp.Status, strings.TrimSpace(string(body)))
}

func (a *Adapter) Upload(ctx context.Context, r core.UploadRequest) (core.UploadResult, error) {
	if r.Bucket == "" || r.Key == "" || r.Source == nil {
		return core.UploadResult{}, fmt.Errorf("bucket, object key, and upload source are required")
	}
	resp, u, err := a.request(ctx, http.MethodPut, r.Bucket, r.Key, nil, r.Source, r.ContentType)
	if err != nil {
		return core.UploadResult{}, fmt.Errorf("upload object to Amazon S3: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return core.UploadResult{}, responseError(resp, "upload")
	}
	defer resp.Body.Close()
	if a.publicURL != nil {
		publicURL := *a.publicURL
		publicURL.Path = strings.TrimRight(publicURL.Path, "/") + "/" + r.Key
		u = &publicURL
	}
	return core.UploadResult{URL: u.String()}, nil
}

func (a *Adapter) ListBuckets(ctx context.Context) ([]string, error) {
	resp, _, err := a.request(ctx, http.MethodGet, "", "", nil, nil, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, responseError(resp, "list buckets")
	}
	var result struct {
		Buckets []struct {
			Name string `xml:"Name"`
		} `xml:"Buckets>Bucket"`
	}
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode Amazon S3 buckets: %w", err)
	}
	buckets := make([]string, 0, len(result.Buckets))
	for _, bucket := range result.Buckets {
		buckets = append(buckets, bucket.Name)
	}
	return buckets, nil
}

func (a *Adapter) Get(ctx context.Context, r core.GetRequest) (core.GetResult, error) {
	if r.Bucket == "" || r.Key == "" {
		return core.GetResult{}, fmt.Errorf("bucket and object key are required")
	}
	resp, u, err := a.request(ctx, http.MethodGet, r.Bucket, r.Key, nil, nil, "")
	if err != nil {
		return core.GetResult{}, err
	}
	if resp.StatusCode/100 != 2 {
		return core.GetResult{}, responseError(resp, "get object")
	}
	return core.GetResult{Object: core.Object{Bucket: r.Bucket, Key: r.Key, URL: u.String(), ContentType: resp.Header.Get("Content-Type"), ETag: strings.Trim(resp.Header.Get("ETag"), "\""), UpdatedAt: parseHTTPTime(resp.Header.Get("Last-Modified"))}, Body: resp.Body}, nil
}

func (a *Adapter) List(ctx context.Context, r core.ListRequest) (core.ListResult, error) {
	if r.Bucket == "" {
		return core.ListResult{}, fmt.Errorf("bucket is required")
	}
	q := url.Values{"list-type": {"2"}, "prefix": {r.Prefix}, "max-keys": {fmt.Sprint(pageLimit(r.Limit))}}
	if r.Cursor != "" {
		q.Set("continuation-token", r.Cursor)
	}
	resp, _, err := a.request(ctx, http.MethodGet, r.Bucket, "", q, nil, "")
	if err != nil {
		return core.ListResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return core.ListResult{}, responseError(resp, "list objects")
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
	result := core.ListResult{}
	for _, obj := range page.Contents {
		result.Objects = append(result.Objects, core.Object{Bucket: r.Bucket, Key: obj.Key, Size: obj.Size, ETag: strings.Trim(obj.ETag, "\""), UpdatedAt: obj.LastModified})
	}
	if page.Truncated {
		result.NextCursor = page.Next
	}
	return result, nil
}

func (a *Adapter) Delete(ctx context.Context, r core.DeleteRequest) error {
	if r.Bucket == "" || r.Key == "" {
		return fmt.Errorf("bucket and object key are required")
	}
	resp, _, err := a.request(ctx, http.MethodDelete, r.Bucket, r.Key, nil, nil, "")
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return responseError(resp, "delete object")
	}
	return resp.Body.Close()
}

func (a *Adapter) DeleteBucket(ctx context.Context, r core.DeleteBucketRequest) error {
	if r.Bucket == "" {
		return fmt.Errorf("bucket is required")
	}
	resp, _, err := a.request(ctx, http.MethodDelete, r.Bucket, "", nil, nil, "")
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return responseError(resp, "delete bucket")
	}
	return resp.Body.Close()
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

func parseHTTPTime(value string) time.Time {
	t, _ := time.Parse(http.TimeFormat, value)
	return t
}

var _ core.Adapter = (*Adapter)(nil)
var _ core.ObjectAdapter = (*Adapter)(nil)
var _ core.BucketAdapter = (*Adapter)(nil)
var _ core.BucketDeleter = (*Adapter)(nil)
