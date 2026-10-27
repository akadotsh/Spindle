// Package cloudinary uploads images through Cloudinary's Upload API.
package cloudinary

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/akashprasad/file-uploader/packages/sdk/core"
)

type Config struct {
	CloudName  string
	APIKey     string
	APISecret  string
	HTTPClient *http.Client
}

type Adapter struct {
	cloudName string
	apiKey    string
	apiSecret string
	client    *http.Client
}

func New(config Config) (*Adapter, error) {
	if strings.TrimSpace(config.CloudName) == "" || strings.TrimSpace(config.APIKey) == "" || config.APISecret == "" {
		return nil, fmt.Errorf("Cloudinary cloud name, API key, and API secret are required")
	}
	client := config.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &Adapter{cloudName: config.CloudName, apiKey: config.APIKey, apiSecret: config.APISecret, client: client}, nil
}

func (a *Adapter) Upload(ctx context.Context, request core.UploadRequest) (core.UploadResult, error) {
	if request.Source == nil || request.Key == "" {
		return core.UploadResult{}, fmt.Errorf("upload source and Cloudinary public ID are required")
	}
	// shortcut: uploads use the single-request endpoint; add chunking for files over 100 MB.
	bodyReader, bodyWriter := io.Pipe()
	form := multipart.NewWriter(bodyWriter)
	go func() {
		defer bodyWriter.Close()
		defer form.Close()
		if err := form.WriteField("public_id", request.Key); err != nil {
			bodyWriter.CloseWithError(err)
			return
		}
		part, err := form.CreateFormFile("file", "upload")
		if err == nil {
			_, err = io.Copy(part, request.Source)
		}
		if err != nil {
			bodyWriter.CloseWithError(err)
		}
	}()

	endpoint := "https://api.cloudinary.com/v1_1/" + url.PathEscape(a.cloudName) + "/image/upload"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bodyReader)
	if err != nil {
		bodyReader.CloseWithError(err)
		return core.UploadResult{}, err
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.SetBasicAuth(a.apiKey, a.apiSecret)
	response, err := a.client.Do(req)
	if err != nil {
		bodyReader.CloseWithError(err)
		return core.UploadResult{}, fmt.Errorf("upload image to Cloudinary: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return core.UploadResult{}, fmt.Errorf("Cloudinary upload failed (%s): %s", response.Status, strings.TrimSpace(string(body)))
	}
	var result struct {
		SecureURL string `json:"secure_url"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		return core.UploadResult{}, fmt.Errorf("decode Cloudinary upload response: %w", err)
	}
	if result.SecureURL == "" {
		return core.UploadResult{}, fmt.Errorf("Cloudinary response did not include a secure URL")
	}
	return core.UploadResult{URL: result.SecureURL}, nil
}
