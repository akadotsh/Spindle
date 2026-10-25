// Package sdk provides a unified file upload API.
package sdk

import (
	"context"
	"fmt"

	"github.com/akashprasad/file-uploader/packages/adapters/gcs"
	"github.com/akashprasad/file-uploader/packages/adapters/supabase"
	"github.com/akashprasad/file-uploader/packages/adapters/upstash"
	"github.com/akashprasad/file-uploader/packages/adapters/vercel"
	"github.com/akashprasad/file-uploader/packages/sdk/core"
)

type UploadRequest = core.UploadRequest
type UploadResult = core.UploadResult
type Adapter = core.Adapter

// Provider selects the storage provider.
type Provider string

const (
	ProviderGCS      Provider = "gcs"
	ProviderSupabase Provider = "supabase"
	ProviderUpstash  Provider = "upstash"
	ProviderVercel   Provider = "vercel"
)

type GCSConfig = gcs.Config
type SupabaseConfig = supabase.Config
type UpstashConfig = upstash.Config
type VercelConfig = vercel.Config

// Config selects a provider and its credentials.
type Config struct {
	Provider Provider
	GCS      *GCSConfig
	Supabase *SupabaseConfig
	Upstash  *UpstashConfig
	Vercel   *VercelConfig
}

// Client uploads files to its configured provider.
type Client struct {
	adapter Adapter
}

// New creates a provider client.
func New(config Config) (*Client, error) {
	switch config.Provider {
	case ProviderGCS:
		if config.GCS == nil || config.Supabase != nil || config.Upstash != nil || config.Vercel != nil {
			return nil, fmt.Errorf("provide only GCS configuration when provider is %q", config.Provider)
		}
		adapter, err := gcs.New(*config.GCS)
		if err != nil {
			return nil, err
		}
		return NewWithAdapter(adapter), nil
	case ProviderUpstash:
		if config.Upstash == nil || config.GCS != nil || config.Supabase != nil || config.Vercel != nil {
			return nil, fmt.Errorf("provide only Upstash configuration when provider is %q", config.Provider)
		}
		adapter, err := upstash.New(*config.Upstash)
		if err != nil {
			return nil, err
		}
		return NewWithAdapter(adapter), nil
	case ProviderVercel:
		if config.Vercel == nil || config.GCS != nil || config.Supabase != nil || config.Upstash != nil {
			return nil, fmt.Errorf("provide only Vercel configuration when provider is %q", config.Provider)
		}
		adapter, err := vercel.New(*config.Vercel)
		if err != nil {
			return nil, err
		}
		return NewWithAdapter(adapter), nil
	case ProviderSupabase:
		if config.Supabase == nil || config.GCS != nil || config.Upstash != nil || config.Vercel != nil {
			return nil, fmt.Errorf("provide only Supabase configuration when provider is %q", config.Provider)
		}
		adapter, err := supabase.New(*config.Supabase)
		if err != nil {
			return nil, err
		}
		return NewWithAdapter(adapter), nil
	default:
		return nil, fmt.Errorf("unsupported upload provider %q", config.Provider)
	}
}

// NewWithAdapter creates a client for a custom adapter.
func NewWithAdapter(adapter Adapter) *Client {
	return &Client{adapter: adapter}
}

// Upload sends a file to the configured provider.
func (c *Client) Upload(ctx context.Context, request UploadRequest) (UploadResult, error) {
	if c == nil || c.adapter == nil {
		return UploadResult{}, fmt.Errorf("upload client has no adapter")
	}
	return c.adapter.Upload(ctx, request)
}
