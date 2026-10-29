package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/akashprasad/file-uploader/packages/sdk"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("uploader", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	provider := flags.String("provider", "supabase", "upload provider: supabase, filesystem, neon, uploadthing, cloudinary, or cloudflare-r2")
	bucket := flags.String("bucket", "", "bucket name or filesystem subdirectory")
	key := flags.String("key", "", "object key")
	filePath := flags.String("file", "", "file to upload")
	rootDir := flags.String("root-dir", "./uploads", "filesystem provider root directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	needsBucket := *provider != string(sdk.ProviderUploadThing) && *provider != string(sdk.ProviderCloudinary)
	if (needsBucket && *bucket == "") || *key == "" || *filePath == "" {
		return errors.New("usage: go run ./cmd/uploader --key KEY --file PATH (add --bucket for bucket providers; Cloudinary keys omit the image extension)")
	}
	var config sdk.Config
	switch sdk.Provider(*provider) {
	case sdk.ProviderSupabase:
		projectURL := os.Getenv("SUPABASE_PROJECT_URL")
		apiKey := os.Getenv("SUPABASE_API_KEY")
		if projectURL == "" || apiKey == "" {
			return errors.New("set SUPABASE_PROJECT_URL and SUPABASE_API_KEY")
		}
		supabaseConfig := sdk.SupabaseConfig{ProjectURL: projectURL, APIKey: apiKey}
		if accessToken := os.Getenv("SUPABASE_ACCESS_TOKEN"); accessToken != "" {
			supabaseConfig.TokenSource = func(context.Context) (string, error) { return accessToken, nil }
		}
		config = sdk.Config{Provider: sdk.ProviderSupabase, Supabase: &supabaseConfig}
	case sdk.ProviderNeon:
		neonConfig := sdk.NeonConfig{Endpoint: os.Getenv("AWS_ENDPOINT_URL_S3"), Region: os.Getenv("AWS_REGION"), AccessKeyID: os.Getenv("AWS_ACCESS_KEY_ID"), SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY")}
		config = sdk.Config{Provider: sdk.ProviderNeon, Neon: &neonConfig}
	case sdk.ProviderFilesystem:
		filesystemConfig := sdk.FilesystemConfig{RootDir: *rootDir}
		config = sdk.Config{Provider: sdk.ProviderFilesystem, Filesystem: &filesystemConfig}
	case sdk.ProviderUploadThing:
		token := os.Getenv("UPLOADTHING_TOKEN")
		if token == "" {
			return errors.New("set UPLOADTHING_TOKEN")
		}
		uploadThingConfig := sdk.UploadThingConfig{Token: token}
		config = sdk.Config{Provider: sdk.ProviderUploadThing, UploadThing: &uploadThingConfig}
	case sdk.ProviderCloudinary:
		cloudinaryConfig := sdk.CloudinaryConfig{CloudName: os.Getenv("CLOUDINARY_CLOUD_NAME"), APIKey: os.Getenv("CLOUDINARY_API_KEY"), APISecret: os.Getenv("CLOUDINARY_API_SECRET")}
		config = sdk.Config{Provider: sdk.ProviderCloudinary, Cloudinary: &cloudinaryConfig}
	case sdk.ProviderCloudflareR2:
		r2Config := sdk.CloudflareR2Config{
			AccountID: os.Getenv("CLOUDFLARE_R2_ACCOUNT_ID"), Bucket: *bucket,
			AccessKeyID: os.Getenv("CLOUDFLARE_R2_ACCESS_KEY_ID"), SecretAccessKey: os.Getenv("CLOUDFLARE_R2_SECRET_ACCESS_KEY"),
			PublicBaseURL: os.Getenv("CLOUDFLARE_R2_PUBLIC_BASE_URL"),
		}
		config = sdk.Config{Provider: sdk.ProviderCloudflareR2, CloudflareR2: &r2Config}
	default:
		return fmt.Errorf("unsupported CLI provider %q", *provider)
	}

	client, err := sdk.New(config)
	if err != nil {
		return err
	}

	file, err := os.Open(*filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	result, err := client.Upload(context.Background(), sdk.UploadRequest{
		Source: file,
		Bucket: *bucket,
		Key:    *key,
	})
	if err != nil {
		return err
	}
	fmt.Println(result.URL)
	return nil
}
