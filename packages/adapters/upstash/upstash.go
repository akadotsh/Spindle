// Package upstash uploads to Upstash Blob.
package upstash

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

// Credentials are short-lived S3 credentials obtained for an Upstash bucket.
type Credentials struct {
	AccessKey    string
	SecretKey    string
	SessionToken string
}

// Config configures the Upstash S3 upload.
type Config struct {
	Endpoint          string
	Region            string
	Bucket            string
	PublicBaseURL     string
	CredentialsSource func(context.Context) (Credentials, error)
	HTTPClient        *http.Client
	Now               func() time.Time
}

// Adapter uploads to Upstash Blob over S3.
type Adapter struct {
	endpoint    *url.URL
	region      string
	bucket      string
	publicBase  *url.URL
	credentials func(context.Context) (Credentials, error)
	client      *http.Client
	now         func() time.Time
}

// New creates an Upstash Blob adapter.
func New(config Config) (*Adapter, error) {
	endpoint, err := url.Parse(config.Endpoint)
	if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" {
		return nil, fmt.Errorf("a valid HTTPS S3 endpoint is required")
	}
	if config.Region == "" || config.Bucket == "" || config.CredentialsSource == nil {
		return nil, fmt.Errorf("region, bucket, and credentials source are required")
	}
	var publicBase *url.URL
	if config.PublicBaseURL != "" {
		publicBase, err = url.Parse(config.PublicBaseURL)
		if err != nil || publicBase.Scheme != "https" || publicBase.Host == "" {
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
	return &Adapter{endpoint, config.Region, config.Bucket, publicBase, config.CredentialsSource, client, now}, nil
}

// Upload writes the object using AWS Signature Version 4.
func (a *Adapter) Upload(ctx context.Context, request core.UploadRequest) (core.UploadResult, error) {
	if request.Source == nil || request.Key == "" {
		return core.UploadResult{}, fmt.Errorf("upload source and object key are required")
	}
	credentials, err := a.credentials(ctx)
	if err != nil {
		return core.UploadResult{}, fmt.Errorf("get Upstash S3 credentials: %w", err)
	}
	if credentials.AccessKey == "" || credentials.SecretKey == "" {
		return core.UploadResult{}, fmt.Errorf("Upstash S3 credentials are incomplete")
	}

	u := *a.endpoint
	u.Path = strings.TrimRight(u.Path, "/") + "/" + a.bucket + "/" + request.Key
	stamp := a.now().UTC()
	date, amzDate := stamp.Format("20060102"), stamp.Format("20060102T150405Z")
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u.String(), request.Source)
	if err != nil {
		return core.UploadResult{}, fmt.Errorf("create Upstash upload request: %w", err)
	}
	req.Header.Set("x-amz-content-sha256", "UNSIGNED-PAYLOAD")
	req.Header.Set("x-amz-date", amzDate)
	if credentials.SessionToken != "" {
		req.Header.Set("x-amz-security-token", credentials.SessionToken)
	}
	if request.ContentType != "" {
		req.Header.Set("Content-Type", request.ContentType)
	}
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalHeaders := "host:" + u.Host + "\n" +
		"x-amz-content-sha256:UNSIGNED-PAYLOAD\n" + "x-amz-date:" + amzDate + "\n"
	if credentials.SessionToken != "" {
		signedHeaders += ";x-amz-security-token"
		canonicalHeaders += "x-amz-security-token:" + credentials.SessionToken + "\n"
	}
	if request.ContentType != "" {
		signedHeaders = "content-type;" + signedHeaders
		canonicalHeaders = "content-type:" + request.ContentType + "\n" + canonicalHeaders
	}
	canonicalURI := u.EscapedPath()
	canonicalRequest := strings.Join([]string{http.MethodPut, canonicalURI, "", canonicalHeaders, signedHeaders, "UNSIGNED-PAYLOAD"}, "\n")
	hash := sha256.Sum256([]byte(canonicalRequest))
	scope := date + "/" + a.region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + fmt.Sprintf("%x", hash)
	signature := sign(credentials.SecretKey, date, a.region, stringToSign)
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+credentials.AccessKey+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+signature)

	response, err := a.client.Do(req)
	if err != nil {
		return core.UploadResult{}, fmt.Errorf("upload object to Upstash Blob: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return core.UploadResult{}, fmt.Errorf("Upstash S3 upload failed (%s): %s", response.Status, strings.TrimSpace(string(body)))
	}
	objectURL := u
	if a.publicBase != nil {
		objectURL = *a.publicBase
		objectURL.Path = strings.TrimRight(objectURL.Path, "/") + "/" + request.Key
	}
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

func (a *Adapter) signedRequest(ctx context.Context, method, key string, query url.Values) (*http.Response, error) {
	credentials, err := a.credentials(ctx)
	if err != nil {
		return nil, fmt.Errorf("get Upstash S3 credentials: %w", err)
	}
	if credentials.AccessKey == "" || credentials.SecretKey == "" {
		return nil, fmt.Errorf("Upstash S3 credentials are incomplete")
	}
	u := *a.endpoint
	u.Path = strings.TrimRight(u.Path, "/") + "/" + a.bucket
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
	if credentials.SessionToken != "" {
		req.Header.Set("x-amz-security-token", credentials.SessionToken)
		signedHeaders += ";x-amz-security-token"
		canonicalHeaders += "x-amz-security-token:" + credentials.SessionToken + "\n"
	}
	canonicalQuery := strings.ReplaceAll(u.Query().Encode(), "+", "%20")
	canonicalRequest := strings.Join([]string{method, u.EscapedPath(), canonicalQuery, canonicalHeaders, signedHeaders, "UNSIGNED-PAYLOAD"}, "\n")
	hash := sha256.Sum256([]byte(canonicalRequest))
	scope := date + "/" + a.region + "/s3/aws4_request"
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + fmt.Sprintf("%x", hash)
	signature := sign(credentials.SecretKey, date, a.region, toSign)
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+credentials.AccessKey+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+signature)
	return a.client.Do(req)
}

func (a *Adapter) Get(ctx context.Context, r core.GetRequest) (core.GetResult, error) {
	resp, err := a.signedRequest(ctx, http.MethodGet, r.Key, nil)
	if err != nil {
		return core.GetResult{}, err
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return core.GetResult{}, fmt.Errorf("Upstash S3 get failed (%s): %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return core.GetResult{Object: core.Object{Bucket: a.bucket, Key: r.Key, ContentType: resp.Header.Get("Content-Type"), ETag: resp.Header.Get("ETag")}, Body: resp.Body}, nil
}

func (a *Adapter) List(ctx context.Context, r core.ListRequest) (core.ListResult, error) {
	q := url.Values{"list-type": {"2"}, "prefix": {r.Prefix}, "max-keys": {fmt.Sprint(upstashPageLimit(r.Limit))}}
	if r.Cursor != "" {
		q.Set("continuation-token", r.Cursor)
	}
	resp, err := a.signedRequest(ctx, http.MethodGet, "", q)
	if err != nil {
		return core.ListResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return core.ListResult{}, fmt.Errorf("Upstash S3 list failed (%s): %s", resp.Status, strings.TrimSpace(string(b)))
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
		out.Objects = append(out.Objects, core.Object{Bucket: a.bucket, Key: obj.Key, Size: obj.Size, ETag: strings.Trim(obj.ETag, "\""), UpdatedAt: obj.LastModified})
	}
	if page.Truncated {
		out.NextCursor = page.Next
	}
	return out, nil
}

func (a *Adapter) Delete(ctx context.Context, r core.DeleteRequest) error {
	resp, err := a.signedRequest(ctx, http.MethodDelete, r.Key, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Upstash S3 delete failed (%s): %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return nil
}

func upstashPageLimit(n int) int {
	if n <= 0 {
		return 100
	}
	if n > 1000 {
		return 1000
	}
	return n
}

var _ core.ObjectAdapter = (*Adapter)(nil)
