# Google Cloud Storage adapter

The `gcs` package implements `sdk.Adapter` using the Google Cloud Storage JSON API and Go's standard HTTP client. It has no Google client library dependency. Supply a token source that returns an OAuth 2.0 access token with permission to create objects in the bucket.

Select `sdk.ProviderGCS` in `sdk.Config` and set its `GCS` field. The top-level SDK constructs the adapter.

The returned URL identifies the object; whether it can be opened anonymously depends on the bucket's access policy.
