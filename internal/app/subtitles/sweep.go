package subtitles

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Sweep cleans the cache: it removes the entries of unknown files and the leftover *.tmp files
// of crashed conversions, then trims the cache to MaxCacheBytes. Call it once at startup. It
// continues past a failing removal and reports the first error.
func (service *Service) Sweep(ctx context.Context) error {
	entries, err := service.dependencies.Cache.List(ctx, service.cacheRoot())
	if err != nil {
		return fmt.Errorf("sweep subtitle cache: %w", err)
	}

	var firstErr error

	remember := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("sweep subtitle cache: %w", err)
		}
	}

	known := map[string]bool{}
	now := service.dependencies.Now()

	for _, entry := range entries {
		remove, err := service.sweepable(ctx, entry, known, now)
		remember(err)

		if remove {
			remember(service.dependencies.Cache.Remove(ctx, entry.Path))
		}
	}

	remember(service.trim(ctx, ""))

	return firstErr
}

// sweepable reports an entry to delete: a stale temp file or a file of an unknown media file.
// known memoizes the lookup per file ID.
func (service *Service) sweepable(ctx context.Context, entry CacheEntry, known map[string]bool, now time.Time) (bool, error) {
	if strings.HasSuffix(entry.Name, tempExtension) {
		return now.Sub(entry.ModTime) > staleTempAge, nil
	}

	exists, cached := known[entry.Directory]
	if !cached {
		var err error

		exists, err = service.dependencies.Files.FileKnown(ctx, entry.Directory)
		if err != nil {
			return false, fmt.Errorf("look up file %s: %w", entry.Directory, err)
		}

		known[entry.Directory] = exists
	}

	return !exists, nil
}

// trim deletes the oldest entries until the cache fits MaxCacheBytes. keep is never deleted
// (the file just written); temp files do not count.
func (service *Service) trim(ctx context.Context, keep string) error {
	service.trimMutex.Lock()
	defer service.trimMutex.Unlock()

	entries, err := service.dependencies.Cache.List(ctx, service.cacheRoot())
	if err != nil {
		return fmt.Errorf("list cache: %w", err)
	}

	entries = slices.DeleteFunc(entries, func(entry CacheEntry) bool {
		return strings.HasSuffix(entry.Name, tempExtension)
	})
	slices.SortFunc(entries, func(left, right CacheEntry) int { return left.ModTime.Compare(right.ModTime) })

	var total int64
	for _, entry := range entries {
		total += entry.Size
	}

	for _, entry := range entries {
		if total <= MaxCacheBytes {
			break
		}

		if entry.Path == keep {
			continue
		}

		if err := service.dependencies.Cache.Remove(ctx, entry.Path); err != nil {
			return fmt.Errorf("evict %s: %w", entry.Name, err)
		}

		total -= entry.Size
	}

	return nil
}
