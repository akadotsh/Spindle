# Spindle

![Spindle logo](assets/spindle.png)

A provider-neutral Go SDK and terminal app for uploading files to local or cloud storage.

## Layout

- `packages/sdk`: typed upload request/result, provider adapter contract, and client.
- `packages/adapters/filesystem`: local filesystem adapter.
- `packages/adapters/gcs`: Google Cloud Storage adapter.
- `packages/adapters/supabase`: Supabase Storage HTTP adapter.
- `packages/adapters/neon`: Neon Object Storage adapter using its S3-compatible API.
- `packages/adapters/upstash`: Upstash Blob adapter using its S3-compatible API.
- `packages/adapters/vercel`: Vercel Blob adapter using its HTTP API.
- `packages/adapters/cloudinary`: Cloudinary image upload adapter using the Upload API.
- `packages/adapters/cloudflare`: Cloudflare R2 adapter using its S3-compatible API.

## Current API

Create a `sdk.Client` with `sdk.New(sdk.Config{Provider: ..., ...})`, then use `Upload`, `Get`, `List`, and `Delete`; call `ListBuckets` to retrieve visible bucket or store names and `DeleteBucket` to delete an empty bucket where supported. Bucket deletion is available for filesystem, GCS, and Supabase adapters. Uploading an existing key replaces it where the provider permits it; Supabase replacement is controlled by its `Upsert` setting. `Get` returns a streaming body that the caller must close. `List` returns object metadata a page at a time, and does not download object contents. Provider-specific configuration is typed and selected in the same config. GCS calls Google's JSON API through standard HTTP, with an injectable OAuth access token source. A standard API key alone does not authorize object writes; an object's URL also does not imply public access.

UploadThing uploads use the dashboard's `UPLOADTHING_TOKEN` and the REST API. UploadThing is application scoped, so the adapter supports uploads but not bucket browsing or object CRUD yet. The `--key` value is used as the uploaded file name. The upload source is staged in temporary files so the adapter can provide the file size required by UploadThing.

Cloudinary uploads use `CLOUDINARY_CLOUD_NAME`, `CLOUDINARY_API_KEY`, and `CLOUDINARY_API_SECRET` with HTTP Basic authentication. Cloudinary is asset based, so the `--key` value is used as the image public ID (omit the file extension); it does not support bucket browsing or object CRUD through this adapter. This adapter uses Cloudinary's single-request image endpoint, which supports uploads up to 100 MB.

```sh
export CLOUDINARY_CLOUD_NAME="your-cloud-name"
export CLOUDINARY_API_KEY="your-api-key"
export CLOUDINARY_API_SECRET="your-api-secret"
go run ./cmd/uploader --provider cloudinary --key images/logo --file ./logo.png
```

Neon Object Storage uses branch scoped S3 credentials. Set `AWS_ENDPOINT_URL_S3`, `AWS_REGION`, `AWS_ACCESS_KEY_ID`, and `AWS_SECRET_ACCESS_KEY` from Neon, then choose a bucket with `--bucket`. Credentials need `storage:read` and `storage:write` scopes for browsing, uploading, downloading, and deleting objects.

```sh
export AWS_ENDPOINT_URL_S3="https://<branch-storage-endpoint>"
export AWS_REGION="us-east-2"
export AWS_ACCESS_KEY_ID="<credential token id>"
export AWS_SECRET_ACCESS_KEY="<credential secret>"
go run ./cmd/uploader --provider neon --bucket close-risk --key example.txt --file ./example.txt
```

Cloudflare R2 uses its S3-compatible API, with region `auto`. Create an R2 API token with object read/write access, then set the account ID and S3 credentials. The bucket is selected with `--bucket`. Set `CLOUDFLARE_R2_PUBLIC_BASE_URL` to a custom domain or public bucket URL if you want the returned URL to be directly accessible; without it, the adapter returns the R2 API endpoint URL.

```sh
export CLOUDFLARE_R2_ACCOUNT_ID="your-account-id"
export CLOUDFLARE_R2_ACCESS_KEY_ID="your-access-key-id"
export CLOUDFLARE_R2_SECRET_ACCESS_KEY="your-secret-access-key"
go run ./cmd/uploader --provider cloudflare-r2 --bucket assets --key images/logo.png --file ./logo.png
```

```sh
export UPLOADTHING_TOKEN="<dashboard token>"
go run ./cmd/uploader --provider uploadthing --key logo.png --file ./logo.png
```

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

The Bubble Tea app supports uploads and object browsing for filesystem, Google Cloud Storage, Supabase, Neon Object Storage, Cloudflare R2, Upstash Blob, and Vercel Blob. Upload-only providers include UploadThing and Cloudinary. Browse first lists available buckets or stores, then lists object metadata in the selected bucket. Use the mouse or arrow keys to select providers, actions, buckets, and objects. Select an object to download it into the current directory, preview it with `p`, or delete it after confirmation. Image previews use Kitty graphics in Kitty, Ghostty, or WezTerm terminals; other terminals show a Unicode half-block preview. PNG, JPEG, and GIF previews are limited to 8 MiB and 20 megapixels. GCS setup asks for the project ID because bucket discovery needs it. Cloudflare R2 and Upstash show the configured bucket; Neon lists buckets on the configured branch; Vercel shows the configured store. Secret fields are masked and provider profiles are stored in the operating system credential store (Keychain on macOS). Press `ctrl+e` on the upload form to update a saved profile.

Google Cloud Storage requires an OAuth access token, which expires and may need updating. Upstash requires S3-compatible credentials for the selected bucket; its bucket token must first be exchanged for those credentials.

```sh
go run ./cmd/tui
```

Supabase values can be entered in the app or prefilled with `SUPABASE_PROJECT_URL` and `SUPABASE_API_KEY`.
