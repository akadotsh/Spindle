# File Uploader SDK for Go

A provider-neutral Go SDK for uploading files to cloud object storage.

## Layout

- `packages/sdk`: typed upload request/result, provider adapter contract, and client.
- `packages/adapters/gcs`: Google Cloud Storage adapter.
- `packages/adapters/s3`: reserved for a reusable S3-compatible adapter.
- `packages/adapters/upstash`: Upstash Blob adapter using its S3-compatible API.
- `packages/adapters/vercel`: Vercel Blob adapter using its HTTP API.

## Current API

Create a `sdk.Client` with `sdk.New(sdk.Config{Provider: ..., ...})`, then call `Upload` with a reader and object destination. Provider-specific configuration is typed and selected in the same config. The GCS adapter calls Google's JSON API through standard HTTP, with an injectable OAuth access token source. A standard API key alone does not authorize object writes; an object's URL also does not imply public access.

```go
client, err := sdk.New(sdk.Config{
	Provider: sdk.ProviderGCS,
	GCS: &sdk.GCSConfig{
		TokenSource: tokenSource,
	},
})
if err != nil {
	return err
}

result, err := client.Upload(ctx, sdk.UploadRequest{
	Source: file,
	Bucket: "my-bucket",
	Key:    "uploads/logo.png",
})
```

## Build in chunks

1. Define the shared SDK contract and unified client configuration. *(complete)*
2. Implement the Google Cloud Storage adapter. *(complete)*
3. Implement Upstash Blob's S3-compatible upload adapter. *(complete)*
4. Implement Vercel Blob's HTTP API adapter. *(complete)*
5. Add integration examples and provider-backed tests.
