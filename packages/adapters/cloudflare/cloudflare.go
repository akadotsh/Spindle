// Package cloudflare adapts Cloudflare R2 to the SDK's S3-compatible transport.
package cloudflare

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/akashprasad/file-uploader/packages/adapters/upstash"
)

type Config struct {
	AccountID       string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	PublicBaseURL   string
	HTTPClient      *http.Client
	Now             func() time.Time
}

type Adapter = upstash.Adapter

func New(config Config) (*Adapter, error) {
	if config.AccountID == "" || strings.IndexFunc(config.AccountID, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-')
	}) >= 0 {
		return nil, fmt.Errorf("a valid Cloudflare account ID is required")
	}
	if config.Bucket == "" || config.AccessKeyID == "" || config.SecretAccessKey == "" {
		return nil, fmt.Errorf("Cloudflare R2 bucket, access key ID, and secret access key are required")
	}
	endpoint := (&url.URL{Scheme: "https", Host: config.AccountID + ".r2.cloudflarestorage.com"}).String()
	return upstash.New(upstash.Config{
		Endpoint:      endpoint,
		Region:        "auto",
		Bucket:        config.Bucket,
		PublicBaseURL: config.PublicBaseURL,
		HTTPClient:    config.HTTPClient,
		Now:           config.Now,
		CredentialsSource: func(_ context.Context) (upstash.Credentials, error) {
			return upstash.Credentials{AccessKey: config.AccessKeyID, SecretKey: config.SecretAccessKey}, nil
		},
	})
}
