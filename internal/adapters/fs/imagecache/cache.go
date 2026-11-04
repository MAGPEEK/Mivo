// Package imagecache keeps rendered artwork on disk in
// <images>/<id[0:2]>/<id>/<tag>-<width>.<ext> (ADR 0027 §8). Every entry is written to a
// temporary file in its directory and renamed, so readers never see a partial picture.
package imagecache

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/magpeek/mivo/internal/app/images"
	"github.com/magpeek/mivo/internal/domain"
)

const (
	// directoryMode keeps the cache private to the server account.
	directoryMode fs.FileMode = 0o750
	// tempPattern names in-progress writes; the purge recognises them by the prefix.
	tempPattern = tempPrefix + "*"
	tempPrefix  = ".tmp-"
)

// Cache is the disk cache of rendered pictures. It is safe for concurrent use.
type Cache struct {
	root   string
	logger *slog.Logger
	retry  retryPolicy
}

// New returns a cache in the absolute directory root (the "images" directory below the
// cache path, ADR 0018), created on first use. logger may be nil.
func New(root string, logger *slog.Logger) (*Cache, error) {
	return newCache(root, logger, defaultRetryPolicy())
}

func newCache(root string, logger *slog.Logger, retry retryPolicy) (*Cache, error) {
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("image cache root %q: must be an absolute path: %w", root, domain.ErrInvalid)
	}

	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	return &Cache{root: filepath.Clean(root), logger: logger, retry: retry}, nil
}

// Open returns the cached picture of key. A missing entry wraps domain.ErrNotFound.
func (cache *Cache) Open(_ context.Context, key images.Key) (images.Cached, error) {
	if err := validateKey(key); err != nil {
		return images.Cached{}, err
	}

	path := filepath.Join(entryDirectory(cache.root, key.ID), fileName(key))

	file, err := os.Open(path) //nolint:gosec // path is built from validated parts below root
	if errors.Is(err, fs.ErrNotExist) {
		return images.Cached{}, fmt.Errorf("open cached image %s: %w", fileName(key), domain.ErrNotFound)
	}

	if err != nil {
		return images.Cached{}, fmt.Errorf("open cached image %s: %w", fileName(key), err)
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()

		return images.Cached{}, fmt.Errorf("stat cached image %s: %w", fileName(key), err)
	}

	return images.Cached{Content: file, Size: info.Size()}, nil
}

// Store writes data under key atomically.
func (cache *Cache) Store(ctx context.Context, key images.Key, data []byte) error {
	if err := validateKey(key); err != nil {
		return err
	}

	directory := entryDirectory(cache.root, key.ID)

	temp, err := cache.createTemp(directory)
	if err != nil {
		return fmt.Errorf("store cached image %s: %w", fileName(key), err)
	}

	tempPath := temp.Name()

	if err := writeAndClose(temp, data); err != nil {
		_ = os.Remove(tempPath)

		return fmt.Errorf("store cached image %s: %w", fileName(key), err)
	}

	target := filepath.Join(directory, fileName(key))

	err = withRetry(ctx, cache.retry, func() error { return os.Rename(tempPath, target) })
	if err != nil {
		_ = os.Remove(tempPath)

		return fmt.Errorf("store cached image %s: rename: %w", fileName(key), err)
	}

	return nil
}

// createTemp creates the entry directory and a temporary file in it. A concurrent purge may
// remove the just created empty directory, so a vanished directory is created once more.
func (cache *Cache) createTemp(directory string) (*os.File, error) {
	var lastErr error

	for range createAttempts {
		if err := os.MkdirAll(directory, directoryMode); err != nil {
			return nil, fmt.Errorf("create directory: %w", err)
		}

		temp, err := os.CreateTemp(directory, tempPattern)
		if err == nil {
			return temp, nil
		}

		lastErr = err
		if !errors.Is(err, fs.ErrNotExist) {
			break
		}
	}

	return nil, fmt.Errorf("create temporary file: %w", lastErr)
}

// createAttempts bounds the directory re-creation race with the purge.
const createAttempts = 3

func writeAndClose(file *os.File, data []byte) error {
	if _, err := file.Write(data); err != nil {
		_ = file.Close()

		return fmt.Errorf("write: %w", err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}

	return nil
}
