// Package filesystem stores uploads under a local directory.
package filesystem

import (
	"context"
	"fmt"
	"io"
	"io/fs"
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

func (a *Adapter) Get(ctx context.Context, request core.GetRequest) (core.GetResult, error) {
	name, err := a.objectPath(request.Bucket, request.Key)
	if err != nil {
		return core.GetResult{}, err
	}
	root, err := os.OpenRoot(a.rootDir)
	if err != nil {
		return core.GetResult{}, err
	}
	file, err := root.Open(name)
	if err != nil {
		root.Close()
		return core.GetResult{}, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		root.Close()
		return core.GetResult{}, err
	}
	return core.GetResult{Object: core.Object{Bucket: request.Bucket, Key: request.Key, Size: info.Size(), UpdatedAt: info.ModTime()}, Body: &rootFile{File: file, root: root}}, nil
}

type rootFile struct {
	*os.File
	root *os.Root
}

func (f *rootFile) Close() error {
	err := f.File.Close()
	if e := f.root.Close(); err == nil {
		err = e
	}
	return err
}

func (a *Adapter) List(ctx context.Context, request core.ListRequest) (core.ListResult, error) {
	if request.Bucket == "" || !filepath.IsLocal(filepath.FromSlash(request.Bucket)) {
		return core.ListResult{}, fmt.Errorf("bucket must be a relative path")
	}
	base, err := a.objectPath(request.Bucket, "_")
	if err != nil {
		return core.ListResult{}, err
	}
	base = strings.TrimSuffix(base, "_")
	limit := request.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	var all []core.Object
	err = filepath.WalkDir(filepath.Join(a.rootDir, base), func(file string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(filepath.Join(a.rootDir, request.Bucket), file)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if strings.HasPrefix(key, request.Prefix) && key > request.Cursor {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			all = append(all, core.Object{Bucket: request.Bucket, Key: key, Size: info.Size(), UpdatedAt: info.ModTime()})
		}
		return nil
	})
	if err != nil {
		return core.ListResult{}, err
	}
	// WalkDir is lexical, so the cursor can be applied before returning the page.
	result := core.ListResult{}
	for i, object := range all {
		if i == limit {
			result.NextCursor = all[i-1].Key
			break
		}
		result.Objects = append(result.Objects, object)
	}
	return result, nil
}

func (a *Adapter) Delete(ctx context.Context, request core.DeleteRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	name, err := a.objectPath(request.Bucket, request.Key)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(a.rootDir)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.Remove(name)
}

func (a *Adapter) objectPath(bucket, key string) (string, error) {
	if bucket == "" || key == "" || !filepath.IsLocal(filepath.FromSlash(bucket)) || !filepath.IsLocal(filepath.FromSlash(key)) || bucket == "." || key == "." {
		return "", fmt.Errorf("bucket and key must be relative paths")
	}
	return path.Join(bucket, key), nil
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
