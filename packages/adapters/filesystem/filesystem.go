// Package filesystem stores uploads under a local directory.
package filesystem

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/akashprasad/file-uploader/packages/sdk/core"
)

type Config struct {
	RootDir string
}

type Adapter struct {
	rootDir string
}

func New(config Config) (*Adapter, error) {
	if config.RootDir == "" {
		return nil, fmt.Errorf("filesystem root directory is required")
	}
	rootDir, err := filepath.Abs(config.RootDir)
	if err != nil {
		return nil, fmt.Errorf("resolve filesystem root directory: %w", err)
	}
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return nil, fmt.Errorf("create filesystem root directory: %w", err)
	}
	return &Adapter{rootDir: rootDir}, nil
}

func (a *Adapter) Upload(ctx context.Context, request core.UploadRequest) (core.UploadResult, error) {
	if err := ctx.Err(); err != nil {
		return core.UploadResult{}, err
	}
	if request.Source == nil {
		return core.UploadResult{}, fmt.Errorf("upload source is required")
	}
	if request.Bucket == "" || request.Key == "" || !filepath.IsLocal(filepath.FromSlash(request.Bucket)) || !filepath.IsLocal(filepath.FromSlash(request.Key)) || request.Bucket == "." || request.Key == "." {
		return core.UploadResult{}, fmt.Errorf("bucket and key must be relative paths")
	}

	destination := path.Join(request.Bucket, request.Key)
	root, err := os.OpenRoot(a.rootDir)
	if err != nil {
		return core.UploadResult{}, fmt.Errorf("open filesystem root: %w", err)
	}
	defer root.Close()

	if err := mkdirAll(root, path.Dir(destination)); err != nil {
		return core.UploadResult{}, fmt.Errorf("create destination directory: %w", err)
	}
	file, err := root.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return core.UploadResult{}, fmt.Errorf("open destination: %w", err)
	}

	if _, err := io.Copy(file, request.Source); err != nil {
		file.Close()
		return core.UploadResult{}, fmt.Errorf("write upload: %w", err)
	}
	if err := file.Close(); err != nil {
		return core.UploadResult{}, fmt.Errorf("close upload: %w", err)
	}

	absoluteDestination := filepath.Join(a.rootDir, filepath.FromSlash(destination))
	return core.UploadResult{URL: (&url.URL{Scheme: "file", Path: absoluteDestination}).String()}, nil
}

func mkdirAll(root *os.Root, directory string) error {
	if directory == "." {
		return nil
	}
	current := ""
	for _, segment := range strings.Split(directory, "/") {
		if segment == "" || segment == "." {
			continue
		}
		current = path.Join(current, segment)
		if err := root.Mkdir(current, 0o755); err != nil {
			if _, statErr := root.Stat(current); statErr != nil {
				return err
			}
		}
	}
	return nil
}
