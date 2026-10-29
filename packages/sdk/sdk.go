// Package sdk provides a unified file upload API.
package sdk

import (
	"context"
	"fmt"

	"github.com/akashprasad/file-uploader/packages/adapters/cloudflare"
	"github.com/akashprasad/file-uploader/packages/adapters/cloudinary"
	"github.com/akashprasad/file-uploader/packages/adapters/filesystem"
	"github.com/akashprasad/file-uploader/packages/adapters/gcs"
	"github.com/akashprasad/file-uploader/packages/adapters/neon"
	"github.com/akashprasad/file-uploader/packages/adapters/supabase"
	"github.com/akashprasad/file-uploader/packages/adapters/uploadthing"
	"github.com/akashprasad/file-uploader/packages/adapters/upstash"
	"github.com/akashprasad/file-uploader/packages/adapters/vercel"
	"github.com/akashprasad/file-uploader/packages/sdk/core"
)

type UploadRequest = core.UploadRequest
type UploadResult = core.UploadResult
type Adapter = core.Adapter
type Object = core.Object
type GetRequest = core.GetRequest
type GetResult = core.GetResult
type ListRequest = core.ListRequest
type ListResult = core.ListResult
type DeleteRequest = core.DeleteRequest
type DeleteBucketRequest = core.DeleteBucketRequest
type ObjectAdapter = core.ObjectAdapter
type BucketAdapter = core.BucketAdapter
type BucketDeleter = core.BucketDeleter

// Provider selects the storage provider.
type Provider string

const (
	ProviderGCS          Provider = "gcs"
	ProviderFilesystem   Provider = "filesystem"
	ProviderSupabase     Provider = "supabase"
	ProviderUpstash      Provider = "upstash"
	ProviderNeon         Provider = "neon"
	ProviderUploadThing  Provider = "uploadthing"
	ProviderVercel       Provider = "vercel"
	ProviderCloudinary   Provider = "cloudinary"
	ProviderCloudflareR2 Provider = "cloudflare-r2"
)

type GCSConfig = gcs.Config
type FilesystemConfig = filesystem.Config
type SupabaseConfig = supabase.Config
type UpstashConfig = upstash.Config
type UpstashCredentials = upstash.Credentials
type NeonConfig = neon.Config
type UploadThingConfig = uploadthing.Config
type VercelConfig = vercel.Config
type CloudinaryConfig = cloudinary.Config
type CloudflareR2Config = cloudflare.Config

// Config selects a provider and its credentials.
type Config struct {
	Provider     Provider
	GCS          *GCSConfig
	Filesystem   *FilesystemConfig
	Supabase     *SupabaseConfig
	Upstash      *UpstashConfig
	Neon         *NeonConfig
	UploadThing  *UploadThingConfig
	Vercel       *VercelConfig
	Cloudinary   *CloudinaryConfig
	CloudflareR2 *CloudflareR2Config
}

// Client uploads files to its configured provider.
type Client struct {
	adapter Adapter
}

// New creates a provider client.
func New(config Config) (*Client, error) {
	switch config.Provider {
	case ProviderFilesystem:
		if config.Filesystem == nil || config.GCS != nil || config.Supabase != nil || config.Upstash != nil || config.Neon != nil || config.UploadThing != nil || config.Vercel != nil || config.Cloudinary != nil || config.CloudflareR2 != nil {
			return nil, fmt.Errorf("provide only filesystem configuration when provider is %q", config.Provider)
		}
		adapter, err := filesystem.New(*config.Filesystem)
		if err != nil {
			return nil, err
		}
		return NewWithAdapter(adapter), nil
	case ProviderGCS:
		if config.GCS == nil || config.Filesystem != nil || config.Supabase != nil || config.Upstash != nil || config.Neon != nil || config.UploadThing != nil || config.Vercel != nil || config.Cloudinary != nil || config.CloudflareR2 != nil {
			return nil, fmt.Errorf("provide only GCS configuration when provider is %q", config.Provider)
		}
		adapter, err := gcs.New(*config.GCS)
		if err != nil {
			return nil, err
		}
		return NewWithAdapter(adapter), nil
	case ProviderUpstash:
		if config.Upstash == nil || config.Filesystem != nil || config.GCS != nil || config.Supabase != nil || config.Neon != nil || config.UploadThing != nil || config.Vercel != nil || config.Cloudinary != nil || config.CloudflareR2 != nil {
			return nil, fmt.Errorf("provide only Upstash configuration when provider is %q", config.Provider)
		}
		adapter, err := upstash.New(*config.Upstash)
		if err != nil {
			return nil, err
		}
		return NewWithAdapter(adapter), nil
	case ProviderNeon:
		if config.Neon == nil || config.Filesystem != nil || config.GCS != nil || config.Supabase != nil || config.Upstash != nil || config.UploadThing != nil || config.Vercel != nil || config.Cloudinary != nil || config.CloudflareR2 != nil {
			return nil, fmt.Errorf("provide only Neon configuration when provider is %q", config.Provider)
		}
		adapter, err := neon.New(*config.Neon)
		if err != nil {
			return nil, err
		}
		return NewWithAdapter(adapter), nil
	case ProviderVercel:
		if config.Vercel == nil || config.Filesystem != nil || config.GCS != nil || config.Supabase != nil || config.Upstash != nil || config.Neon != nil || config.UploadThing != nil || config.Cloudinary != nil || config.CloudflareR2 != nil {
			return nil, fmt.Errorf("provide only Vercel configuration when provider is %q", config.Provider)
		}
		adapter, err := vercel.New(*config.Vercel)
		if err != nil {
			return nil, err
		}
		return NewWithAdapter(adapter), nil
	case ProviderUploadThing:
		if config.UploadThing == nil || config.Filesystem != nil || config.GCS != nil || config.Supabase != nil || config.Upstash != nil || config.Neon != nil || config.Vercel != nil || config.Cloudinary != nil || config.CloudflareR2 != nil {
			return nil, fmt.Errorf("provide only UploadThing configuration when provider is %q", config.Provider)
		}
		adapter, err := uploadthing.New(*config.UploadThing)
		if err != nil {
			return nil, err
		}
		return NewWithAdapter(adapter), nil
	case ProviderSupabase:
		if config.Supabase == nil || config.Filesystem != nil || config.GCS != nil || config.Upstash != nil || config.Neon != nil || config.UploadThing != nil || config.Vercel != nil || config.Cloudinary != nil || config.CloudflareR2 != nil {
			return nil, fmt.Errorf("provide only Supabase configuration when provider is %q", config.Provider)
		}
		adapter, err := supabase.New(*config.Supabase)
		if err != nil {
			return nil, err
		}
		return NewWithAdapter(adapter), nil
	case ProviderCloudinary:
		if config.Cloudinary == nil || config.Filesystem != nil || config.GCS != nil || config.Supabase != nil || config.Upstash != nil || config.Neon != nil || config.UploadThing != nil || config.Vercel != nil || config.CloudflareR2 != nil {
			return nil, fmt.Errorf("provide only Cloudinary configuration when provider is %q", config.Provider)
		}
		adapter, err := cloudinary.New(*config.Cloudinary)
		if err != nil {
			return nil, err
		}
		return NewWithAdapter(adapter), nil
	case ProviderCloudflareR2:
		if config.CloudflareR2 == nil || config.Filesystem != nil || config.GCS != nil || config.Supabase != nil || config.Upstash != nil || config.Neon != nil || config.UploadThing != nil || config.Vercel != nil || config.Cloudinary != nil {
			return nil, fmt.Errorf("provide only Cloudflare R2 configuration when provider is %q", config.Provider)
		}
		adapter, err := cloudflare.New(*config.CloudflareR2)
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

func (c *Client) objectAdapter() (ObjectAdapter, error) {
	if c == nil || c.adapter == nil {
		return nil, fmt.Errorf("storage client has no adapter")
	}
	a, ok := c.adapter.(ObjectAdapter)
	if !ok {
		return nil, fmt.Errorf("configured adapter does not support object operations")
	}
	return a, nil
}

// Get retrieves an object. The caller must close the returned body.
func (c *Client) Get(ctx context.Context, request GetRequest) (GetResult, error) {
	a, err := c.objectAdapter()
	if err != nil {
		return GetResult{}, err
	}
	return a.Get(ctx, request)
}

// List returns one page of object metadata. Use NextCursor to fetch more.
func (c *Client) List(ctx context.Context, request ListRequest) (ListResult, error) {
	a, err := c.objectAdapter()
	if err != nil {
		return ListResult{}, err
	}
	return a.List(ctx, request)
}

// Delete removes an object.
func (c *Client) Delete(ctx context.Context, request DeleteRequest) error {
	a, err := c.objectAdapter()
	if err != nil {
		return err
	}
	return a.Delete(ctx, request)
}

// ListBuckets returns the buckets or stores visible to the configured provider credentials.
func (c *Client) ListBuckets(ctx context.Context) ([]string, error) {
	if c == nil || c.adapter == nil {
		return nil, fmt.Errorf("storage client has no adapter")
	}
	a, ok := c.adapter.(BucketAdapter)
	if !ok {
		return nil, fmt.Errorf("configured adapter does not support bucket listing")
	}
	return a.ListBuckets(ctx)
}

// DeleteBucket removes an empty bucket when the provider supports it.
func (c *Client) DeleteBucket(ctx context.Context, request DeleteBucketRequest) error {
	if c == nil || c.adapter == nil {
		return fmt.Errorf("storage client has no adapter")
	}
	a, ok := c.adapter.(BucketDeleter)
	if !ok {
		return fmt.Errorf("configured adapter does not support bucket deletion")
	}
	return a.DeleteBucket(ctx, request)
}
