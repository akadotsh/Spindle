# File Uploader SDK for Go

A provider-neutral Go SDK for uploading files to local or cloud storage.

## Layout

- `packages/sdk`: typed upload request/result, provider adapter contract, and client.
- `packages/adapters/filesystem`: local filesystem adapter.
- `packages/adapters/gcs`: Google Cloud Storage adapter.
- `packages/adapters/supabase`: Supabase Storage HTTP adapter.
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
5. Implement Supabase Storage's HTTP API adapter. *(complete)*
6. Add integration examples and provider-backed tests.

## Test a Supabase upload

Create a bucket in Supabase first, then set the project URL and API key in your shell:

```sh
export SUPABASE_PROJECT_URL="https://your-project-ref.supabase.co"
export SUPABASE_API_KEY="sb_secret_..."
go run ./cmd/uploader --bucket assets --key test/logo.png --file ./logo.png
```

Use a publishable key only when the bucket's Storage policies allow the upload. Keep a secret key private; it bypasses Row Level Security. Set `SUPABASE_ACCESS_TOKEN` to a signed-in user's JWT when uploading with that user's permissions.

## Test a filesystem upload

The filesystem adapter stores files under `<root-dir>/<bucket>/<key>`:

```sh
go run ./cmd/uploader --provider filesystem --root-dir ./uploads --bucket local --key test/logo.png --file ./logo.png
```
