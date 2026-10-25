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
	bucket := flags.String("bucket", "", "Supabase bucket name")
	key := flags.String("key", "", "object key")
	filePath := flags.String("file", "", "file to upload")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *bucket == "" || *key == "" || *filePath == "" {
		return errors.New("usage: go run ./cmd/uploader --bucket BUCKET --key OBJECT_KEY --file PATH")
	}
	projectURL := os.Getenv("SUPABASE_PROJECT_URL")
	apiKey := os.Getenv("SUPABASE_API_KEY")
	if projectURL == "" || apiKey == "" {
		return errors.New("set SUPABASE_PROJECT_URL and SUPABASE_API_KEY")
	}

	config := sdk.SupabaseConfig{ProjectURL: projectURL, APIKey: apiKey}
	if accessToken := os.Getenv("SUPABASE_ACCESS_TOKEN"); accessToken != "" {
		config.TokenSource = func(context.Context) (string, error) { return accessToken, nil }
	}
	client, err := sdk.New(sdk.Config{Provider: sdk.ProviderSupabase, Supabase: &config})
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
