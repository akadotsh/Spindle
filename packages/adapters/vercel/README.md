# Vercel Blob adapter

Vercel Blob is backed by S3, but exposes its own Blob API rather than an S3-compatible endpoint. `vercel.go` uploads over Vercel's token-authenticated HTTP API directly, without a Go client library.

Configure the store ID, read/write token, and access mode (`public` or `private`). The API version is configurable because Vercel versions this protocol.

Configure this through `sdk.Config{Provider: sdk.ProviderVercel, Vercel: ...}`; callers don't construct the adapter directly.

See [Vercel Blob documentation](https://vercel.com/docs/vercel-blob).
