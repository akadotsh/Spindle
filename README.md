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

Create a `sdk.Client` with `sdk.New(sdk.Config{Provider: ..., ...})`, then use `Upload`, `Get`, `List`, and `Delete`; call `ListBuckets` to retrieve the visible bucket or store names. Uploading an existing key replaces it where the provider permits it; Supabase replacement is controlled by its `Upsert` setting. `Get` returns a streaming body that the caller must close. `List` returns object metadata a page at a time, and does not download object contents. Provider-specific configuration is typed and selected in the same config. GCS calls Google's JSON API through standard HTTP, with an injectable OAuth access token source. A standard API key alone does not authorize object writes; an object's URL also does not imply public access.

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
page, err := client.List(ctx, sdk.ListRequest{Bucket: "my-bucket", Prefix: "uploads/", Limit: 100})
for _, object := range page.Objects {
	fmt.Println(object.Key, object.Size)
}
if page.NextCursor != "" {
	page, err = client.List(ctx, sdk.ListRequest{Bucket: "my-bucket", Prefix: "uploads/", Limit: 100, Cursor: page.NextCursor})
}
```

## Build in chunks

1. Define the shared SDK contract and unified client configuration. *(complete)*
2. Implement the Google Cloud Storage adapter. *(complete)*
3. Implement Upstash Blob's S3-compatible upload adapter. *(complete)*
4. Implement Vercel Blob's HTTP API adapter. *(complete)*
5. Implement Supabase Storage's HTTP API adapter. *(complete)*
6. Add object get, list, and delete operations across the adapters. *(complete)*
7. Add integration examples and provider-backed tests.

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

## Run the terminal app

The Bubble Tea app supports uploads and object browsing for filesystem, Google Cloud Storage, Supabase, Upstash Blob, and Vercel Blob. Browse first lists available buckets or stores, then lists object metadata in the selected bucket. Select an object to download it into the current directory or delete it after confirmation. GCS setup asks for the project ID because bucket discovery needs it. Upstash uses a bucket scoped token and Vercel uses the store ID entered in setup, so those providers show that configured destination. Secret fields are masked and provider profiles are stored in the operating system credential store (Keychain on macOS). Press `ctrl+e` on the upload form to update a saved profile.

Google Cloud Storage requires an OAuth access token, which expires and may need updating. Upstash requires S3-compatible credentials for the selected bucket; its bucket token must first be exchanged for those credentials.

```sh
go run ./cmd/tui
```

Supabase values can be entered in the app or prefilled with `SUPABASE_PROJECT_URL` and `SUPABASE_API_KEY`.
