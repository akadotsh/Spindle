# Upstash Blob adapter

`upstash.go` uploads with S3 Signature Version 4 using Go's standard library. Configure the bucket's S3 endpoint and region, and provide a credentials source that returns the short-lived S3 credentials for the Upstash bucket token. Upstash bucket tokens are not themselves S3 credentials, so the token exchange and refresh must be supplied by the caller. Set `PublicBaseURL` when the result should be the bucket's public delivery URL; otherwise the result contains the S3 object endpoint.

Configure this through `sdk.Config{Provider: sdk.ProviderUpstash, Upstash: ...}`; callers don't construct the adapter directly.

See [Upstash's S3 compatibility and credential guidance](https://upstash.com/docs/blob/bucket/connecting).
