# UploadThing adapter

This adapter uploads files through UploadThing's REST API; it does not use a Go client library. Configure it with the `UPLOADTHING_TOKEN` value from the UploadThing dashboard. The adapter currently supports uploading only; UploadThing is application scoped and does not expose the bucket model used by the other providers.

The source is staged in temporary files so its size is known before requesting the UploadThing upload form. Temporary files are removed when the upload finishes.
