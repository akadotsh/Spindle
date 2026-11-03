# Spindle

<p align="center">
  <img src="assets/spindle.png" alt="Spindle logo" width="420">
</p>

Spindle is a Go SDK and terminal app for working with object storage across providers.

**Providers:** Amazon S3, Cloudflare R2, Cloudinary, Google Cloud Storage, Neon Object Storage, Supabase, Upstash Blob, UploadThing, Vercel Blob, and local filesystem.

Amazon S3 uses the S3 REST API with AWS Signature Version 4. Set `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, and `AWS_REGION` (or pass `--region`); set `AWS_SESSION_TOKEN` for temporary credentials. The TUI saves credentials in the system keychain and lists buckets allowed by the credentials.

```sh
export AWS_ACCESS_KEY_ID="your-access-key-id"
export AWS_SECRET_ACCESS_KEY="your-secret-access-key"
export AWS_REGION="us-east-1"
go run ./cmd/uploader --provider s3 --bucket assets --key images/logo.png --file ./logo.png
```

Configure the bucket's access policy separately if uploaded objects should be public. `AWS_S3_PUBLIC_BASE_URL` can point to a public bucket URL or custom domain for the returned URL.

