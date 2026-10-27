// Package uploadthing uploads files through the UploadThing REST API.
package uploadthing

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/akashprasad/file-uploader/packages/sdk/core"
)

const defaultAPIEndpoint = "https://api.uploadthing.com"

type Config struct {
	Token       string
	APIEndpoint string
	HTTPClient  *http.Client
}

type Adapter struct {
	apiKey   string
	endpoint string
	client   *http.Client
}

func New(config Config) (*Adapter, error) {
	apiKey, err := apiKeyFromToken(config.Token)
	if err != nil {
		return nil, err
	}
	endpoint := config.APIEndpoint
	if endpoint == "" {
		endpoint = defaultAPIEndpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, fmt.Errorf("a valid HTTPS UploadThing API endpoint is required")
	}
	client := config.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &Adapter{apiKey: apiKey, endpoint: strings.TrimRight(endpoint, "/"), client: client}, nil
}

func apiKeyFromToken(token string) (string, error) {
	if token == "" {
		return "", fmt.Errorf("UploadThing token is required")
	}
	for _, decode := range []func(string) ([]byte, error){base64.RawURLEncoding.DecodeString, base64.URLEncoding.DecodeString, base64.RawStdEncoding.DecodeString, base64.StdEncoding.DecodeString} {
		data, err := decode(token)
		if err != nil {
			continue
		}
		var credentials struct {
			APIKey string `json:"apiKey"`
		}
		if json.Unmarshal(data, &credentials) == nil && credentials.APIKey != "" {
			return credentials.APIKey, nil
		}
	}
	if strings.HasPrefix(token, "sk_") {
		return token, nil
	}
	return "", fmt.Errorf("UploadThing token must be a dashboard token or API key")
}

func (a *Adapter) Upload(ctx context.Context, request core.UploadRequest) (core.UploadResult, error) {
	if request.Source == nil || request.Key == "" {
		return core.UploadResult{}, fmt.Errorf("upload source and file name are required")
	}

	source, err := os.CreateTemp("", "uploadthing-source-*")
	if err != nil {
		return core.UploadResult{}, fmt.Errorf("create upload temporary file: %w", err)
	}
	defer os.Remove(source.Name())
	defer source.Close()
	size, err := io.Copy(source, request.Source)
	if err != nil {
		return core.UploadResult{}, fmt.Errorf("read upload source: %w", err)
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return core.UploadResult{}, err
	}
	contentType := request.ContentType
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(request.Key))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	filename := path.Base(strings.ReplaceAll(request.Key, "\\", "/"))

	prepareBody, err := json.Marshal(map[string]any{
		"files":              []map[string]any{{"name": filename, "size": size, "type": contentType}},
		"contentDisposition": "inline",
		"acl":                "public-read",
	})
	if err != nil {
		return core.UploadResult{}, err
	}
	prepareRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint+"/v6/uploadFiles", strings.NewReader(string(prepareBody)))
	if err != nil {
		return core.UploadResult{}, err
	}
	prepareRequest.Header.Set("Content-Type", "application/json")
	prepareRequest.Header.Set("x-uploadthing-api-key", a.apiKey)
	prepareResponse, err := a.client.Do(prepareRequest)
	if err != nil {
		return core.UploadResult{}, fmt.Errorf("request UploadThing upload URL: %w", err)
	}
	defer prepareResponse.Body.Close()
	if prepareResponse.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(prepareResponse.Body, 4096))
		return core.UploadResult{}, fmt.Errorf("UploadThing prepare upload failed (%s): %s", prepareResponse.Status, strings.TrimSpace(string(body)))
	}
	var prepared struct {
		Data []struct {
			URL     string            `json:"url"`
			FileURL string            `json:"fileUrl"`
			Fields  map[string]string `json:"fields"`
			Error   *struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(prepareResponse.Body, 1<<20)).Decode(&prepared); err != nil {
		return core.UploadResult{}, fmt.Errorf("decode UploadThing upload response: %w", err)
	}
	if len(prepared.Data) != 1 {
		return core.UploadResult{}, fmt.Errorf("UploadThing returned no upload URL")
	}
	file := prepared.Data[0]
	if file.Error != nil {
		return core.UploadResult{}, fmt.Errorf("UploadThing prepare upload failed: %s", file.Error.Message)
	}
	if file.URL == "" || file.FileURL == "" {
		return core.UploadResult{}, fmt.Errorf("UploadThing returned an incomplete upload response")
	}
	uploadURL, err := url.Parse(file.URL)
	if err != nil || uploadURL.Scheme != "https" || uploadURL.Host == "" || uploadURL.User != nil {
		return core.UploadResult{}, fmt.Errorf("UploadThing returned an invalid upload URL")
	}

	bodyFile, err := os.CreateTemp("", "uploadthing-form-*")
	if err != nil {
		return core.UploadResult{}, fmt.Errorf("create multipart temporary file: %w", err)
	}
	defer os.Remove(bodyFile.Name())
	defer bodyFile.Close()
	writer := multipart.NewWriter(bodyFile)
	for name, value := range file.Fields {
		if err := writer.WriteField(name, value); err != nil {
			return core.UploadResult{}, err
		}
	}
	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "file", "filename": filename}))
	partHeader.Set("Content-Type", contentType)
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		return core.UploadResult{}, err
	}
	if _, err := io.Copy(part, source); err != nil {
		return core.UploadResult{}, fmt.Errorf("write UploadThing multipart file: %w", err)
	}
	if err := writer.Close(); err != nil {
		return core.UploadResult{}, err
	}
	bodyInfo, err := bodyFile.Stat()
	if err != nil {
		return core.UploadResult{}, err
	}
	if _, err := bodyFile.Seek(0, io.SeekStart); err != nil {
		return core.UploadResult{}, err
	}
	uploadRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL.String(), bodyFile)
	if err != nil {
		return core.UploadResult{}, err
	}
	uploadRequest.ContentLength = bodyInfo.Size()
	uploadRequest.Header.Set("Content-Type", writer.FormDataContentType())
	uploadRequest.Header.Set("Accept", "application/xml")
	uploadResponse, err := a.client.Do(uploadRequest)
	if err != nil {
		return core.UploadResult{}, fmt.Errorf("upload file to UploadThing: %w", err)
	}
	defer uploadResponse.Body.Close()
	if uploadResponse.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(uploadResponse.Body, 4096))
		return core.UploadResult{}, fmt.Errorf("UploadThing file upload failed (%s): %s", uploadResponse.Status, strings.TrimSpace(string(body)))
	}
	return core.UploadResult{URL: file.FileURL}, nil
}
