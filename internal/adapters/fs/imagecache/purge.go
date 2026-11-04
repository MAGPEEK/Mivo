package imagecache

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// staleTempAge is how old a temporary file must be before the purge treats it as left over
// from a crash. Writes take milliseconds, so an hour never hits a write in progress.
const staleTempAge = time.Hour

// Purge removes every cached picture whose (id, tag) keep rejects, stale temporary files
// and directories left empty. Foreign files are left alone and logged at debug. A file that
// cannot be removed is logged and skipped, so one locked file never stops the purge.
func (cache *Cache) Purge(ctx context.Context, keep func(id, tag string) bool) error {
	prefixes, err := os.ReadDir(cache.root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("purge image cache: %w", err)
	}

	for _, prefix := range prefixes {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("purge image cache: %w", err)
		}

		path := filepath.Join(cache.root, prefix.Name())
		if !prefix.IsDir() || !isPrefix(prefix.Name()) {
			cache.logForeign(ctx, path)

			continue
		}

		cache.purgePrefix(ctx, path, keep)
	}

	return nil
}

func (cache *Cache) purgePrefix(ctx context.Context, prefixPath string, keep func(id, tag string) bool) {
	entries, err := os.ReadDir(prefixPath)
	if err != nil {
		cache.logger.WarnContext(ctx, "reading image cache directory failed",
			slog.String("path", prefixPath), slog.Any("error", err))

		return
	}

	prefix := filepath.Base(prefixPath)

	for _, entry := range entries {
		path := filepath.Join(prefixPath, entry.Name())
		if !entry.IsDir() || !isUUID(entry.Name()) || !strings.HasPrefix(entry.Name(), prefix) {
			cache.logForeign(ctx, path)

			continue
		}

		cache.purgeEntry(ctx, path, entry.Name(), keep)
		cache.removeIfEmpty(path)
	}

	cache.removeIfEmpty(prefixPath)
}

func (cache *Cache) purgeEntry(ctx context.Context, entryPath, id string, keep func(id, tag string) bool) {
	files, err := os.ReadDir(entryPath)
	if err != nil {
		cache.logger.WarnContext(ctx, "reading image cache directory failed",
			slog.String("path", entryPath), slog.Any("error", err))

		return
	}

	for _, file := range files {
		path := filepath.Join(entryPath, file.Name())

		if !cache.isStale(file, id, keep) {
			if _, ok := parseFileName(file.Name()); !ok && !strings.HasPrefix(file.Name(), tempPrefix) {
				cache.logForeign(ctx, path)
			}

			continue
		}

		err := withRetry(ctx, cache.retry, func() error { return os.Remove(path) })
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			cache.logger.WarnContext(ctx, "removing stale cached image failed",
				slog.String("path", path), slog.Any("error", err))
		}
	}
}

// isStale reports whether file is a cache file of a rejected (id, tag) or a temporary file
// left over from a crash.
func (cache *Cache) isStale(file fs.DirEntry, id string, keep func(id, tag string) bool) bool {
	if !file.Type().IsRegular() {
		return false
	}

	if tag, ok := parseFileName(file.Name()); ok {
		return !keep(id, tag)
	}

	if !strings.HasPrefix(file.Name(), tempPrefix) {
		return false
	}

	info, err := file.Info()

	return err == nil && time.Since(info.ModTime()) > staleTempAge
}

// removeIfEmpty removes a directory that holds nothing any more. os.Remove refuses a
// non-empty directory, which is exactly the case to leave alone.
func (cache *Cache) removeIfEmpty(path string) {
	_ = os.Remove(path)
}

func (cache *Cache) logForeign(ctx context.Context, path string) {
	cache.logger.DebugContext(ctx, "ignoring foreign file in the image cache", slog.String("path", path))
}
