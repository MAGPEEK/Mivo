package cachedir

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"

	"github.com/magpeek/mivo/internal/app/subtitles"
	"github.com/magpeek/mivo/internal/domain"
)

// subtitleEntryDepth is the depth of a cached WebVTT: <fileID>/<name>.
const subtitleEntryDepth = 2

// Subtitles is the subtitle cache <cache>/subtitles. It implements subtitles.Cache.
type Subtitles struct {
	root confinedRoot
}

// NewSubtitles returns the cache below the absolute cacheDir. Errors wrap domain.ErrInvalid.
func NewSubtitles(cacheDir string) (*Subtitles, error) {
	root, err := newConfinedRoot(cacheDir, subtitlesDirectoryName)
	if err != nil {
		return nil, fmt.Errorf("new subtitle cache: %w", err)
	}

	return &Subtitles{root: root}, nil
}

// Open returns the cached file at cachePath, which must be <cache>/subtitles/<fileID>/<name>.
// A missing file wraps domain.ErrNotFound, a path elsewhere domain.ErrForbidden.
func (cache *Subtitles) Open(ctx context.Context, cachePath string) (subtitles.CacheFile, error) {
	if err := ctx.Err(); err != nil {
		return subtitles.CacheFile{}, fmt.Errorf("open cached subtitle: %w", err)
	}

	relative, err := cache.root.relative(cachePath, subtitleEntryDepth)
	if err != nil {
		return subtitles.CacheFile{}, err
	}

	opened, err := cache.root.open(false)
	if isAbsent(err) {
		return subtitles.CacheFile{}, fmt.Errorf("cached subtitle: %w", domain.ErrNotFound)
	}

	if err != nil {
		return subtitles.CacheFile{}, err
	}

	defer func() { _ = opened.Close() }()

	file, err := opened.Open(relative)
	if isAbsent(err) {
		return subtitles.CacheFile{}, fmt.Errorf("cached subtitle: %w", domain.ErrNotFound)
	}

	if err != nil {
		return subtitles.CacheFile{}, fmt.Errorf("open cached subtitle: %w", err)
	}

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()

		return subtitles.CacheFile{}, fmt.Errorf("cached subtitle is no regular file: %w", domain.ErrNotFound)
	}

	return subtitles.CacheFile{Content: file, Size: info.Size(), ModTime: info.ModTime()}, nil
}

// List returns every cached file below root, which must be the cache's own directory. A cache
// that does not exist yet is empty. Entries outside the <fileID>/<name> shape are ignored.
func (cache *Subtitles) List(ctx context.Context, root string) ([]subtitles.CacheEntry, error) {
	if filepath.Clean(root) != cache.root.path {
		return nil, fmt.Errorf("list %q: not the subtitle cache: %w", root, domain.ErrForbidden)
	}

	opened, err := cache.root.open(false)
	if isAbsent(err) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	defer func() { _ = opened.Close() }()

	var entries []subtitles.CacheEntry

	walkErr := fs.WalkDir(opened.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", name, err)
		}

		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("list subtitle cache: %w", ctxErr)
		}

		if listed, ok := cache.listed(name, entry); ok {
			entries = append(entries, listed)
		}

		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("list subtitle cache: %w", walkErr)
	}

	return entries, nil
}

// listed describes a regular file at <fileID>/<name>.
func (cache *Subtitles) listed(name string, entry fs.DirEntry) (subtitles.CacheEntry, bool) {
	directory, base := path.Split(name)
	directory = path.Clean(directory)

	if !entry.Type().IsRegular() || !isFileID(directory) {
		return subtitles.CacheEntry{}, false
	}

	info, err := entry.Info()
	if err != nil {
		return subtitles.CacheEntry{}, false
	}

	return subtitles.CacheEntry{
		Path: filepath.Join(cache.root.path, directory, base), Directory: directory, Name: base,
		Size: info.Size(), ModTime: info.ModTime(),
	}, true
}

// Remove deletes the cached file at cachePath and its folder when that became empty. A missing
// file is no error; a path outside <fileID>/<name> wraps domain.ErrForbidden.
func (cache *Subtitles) Remove(ctx context.Context, cachePath string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("remove cached subtitle: %w", err)
	}

	relative, err := cache.root.relative(cachePath, subtitleEntryDepth)
	if err != nil {
		return err
	}

	opened, err := cache.root.open(false)
	if isAbsent(err) {
		return nil
	}

	if err != nil {
		return err
	}

	defer func() { _ = opened.Close() }()

	if err := opened.Remove(relative); err != nil && !isAbsent(err) {
		return fmt.Errorf("remove cached subtitle: %w", err)
	}

	// Fails while other tracks of the file are cached, which is the point.
	_ = opened.Remove(path.Dir(relative))

	return nil
}

var _ subtitles.Cache = (*Subtitles)(nil)
