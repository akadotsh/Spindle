# Supabase Storage adapter

`supabase.go` uploads files through the Supabase Storage HTTP API. Configure the project URL and API key in `sdk.SupabaseConfig`; set `TokenSource` when requests should include a signed-in user's JWT. Each upload request supplies its bucket and object key.

Uploads require an appropriate Storage access policy. `Upsert` controls whether an existing object can be replaced. `Public` controls which object URL is returned; it does not change the bucket's access policy. Create buckets separately in Supabase before uploading.
