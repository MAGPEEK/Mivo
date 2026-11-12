package cachedir

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/app/trickplay"
	"github.com/magpeek/mivo/internal/domain"
)

const (
	// manifestName is the stored manifest next to the sheets of a file.
	manifestName = "manifest.json"
	// manifestTempName is where Save writes before it renames over manifestName.
	manifestTempName = manifestName + ".tmp"
	// manifestMode is the permission of the written manifest.
	manifestMode = 0o640
	// maxManifestBytes bounds what Load reads; a real manifest is a few hundred bytes.
	maxManifestBytes = 64 << 10
	// stagingPrefix starts the staging directories of ffmpeg.TrickplayGenerator.
	stagingPrefix = ".trickplay-"
	// invalidDirectoryName is where Directory points for an ID that is no catalog file ID: a
	// folder that never holds sheets, so a lookup with a bad ID cannot reach another folder.
	invalidDirectoryName = ".invalid"
)

// Trickplay is the trickplay cache <cache>/trickplay. It implements trickplay.Store and
// trickplay.Cache.
type Trickplay struct {
	root confinedRoot
}

// NewTrickplay returns the cache below the absolute cacheDir. Errors wrap domain.ErrInvalid.
func NewTrickplay(cacheDir string) (*Trickplay, error) {
	root, err := newConfinedRoot(cacheDir, trickplayDirectoryName)
	if err != nil {
		return nil, fmt.Errorf("new trickplay cache: %w", err)
	}

	return &Trickplay{root: root}, nil
}

// storedManifest is the JSON form of trickplay.Stored.
type storedManifest struct {
	TileWidth  int    `json:"tileWidth"`
	TileHeight int    `json:"tileHeight"`
	Columns    int    `json:"columns"`
	Rows       int    `json:"rows"`
	IntervalNS int64  `json:"intervalNs"`
	SheetCount int    `json:"sheetCount"`
	Key        string `json:"key"`
}

// Directory returns the absolute directory of a file ID. An ID that is no canonical UUID
// yields a folder that never holds sheets.
func (cache *Trickplay) Directory(fileID string) string {
	if !isFileID(fileID) {
		return filepath.Join(cache.root.path, invalidDirectoryName)
	}

	return filepath.Join(cache.root.path, fileID)
}

func (cache *Trickplay) manifestPath(fileID string) string {
	return fileID + pathSeparator + manifestName
}

// Load returns the stored manifest of a file; an error wrapping domain.ErrNotFound when none
// exists or it is unreadable (it is regenerated then).
func (cache *Trickplay) Load(ctx context.Context, fileID string) (trickplay.Stored, error) {
	if err := ctx.Err(); err != nil {
		return trickplay.Stored{}, fmt.Errorf("load trickplay manifest: %w", err)
	}

	if !isFileID(fileID) {
		return trickplay.Stored{}, fmt.Errorf("trickplay %q: %w", fileID, domain.ErrNotFound)
	}

	opened, err := cache.root.open(false)
	if isAbsent(err) {
		return trickplay.Stored{}, fmt.Errorf("trickplay %s: %w", fileID, domain.ErrNotFound)
	}

	if err != nil {
		return trickplay.Stored{}, err
	}

	defer func() { _ = opened.Close() }()

	return readManifest(opened, cache.manifestPath(fileID), fileID)
}

func readManifest(opened *os.Root, name, fileID string) (trickplay.Stored, error) {
	file, err := opened.Open(name)
	if isAbsent(err) {
		return trickplay.Stored{}, fmt.Errorf("trickplay %s: %w", fileID, domain.ErrNotFound)
	}

	if err != nil {
		return trickplay.Stored{}, fmt.Errorf("open trickplay manifest: %w", err)
	}

	defer func() { _ = file.Close() }()

	var stored storedManifest

	if err := json.NewDecoder(io.LimitReader(file, maxManifestBytes)).Decode(&stored); err != nil {
		return trickplay.Stored{}, fmt.Errorf("trickplay %s: manifest unreadable: %w", fileID, domain.ErrNotFound)
	}

	return trickplay.Stored{
		Key: stored.Key,
		Manifest: trickplay.Manifest{
			TileWidth: stored.TileWidth, TileHeight: stored.TileHeight, Columns: stored.Columns, Rows: stored.Rows,
			Interval: time.Duration(stored.IntervalNS), SheetCount: stored.SheetCount,
		},
	}, nil
}

// Save writes the manifest of a file after its sheets are in place; the file appears complete
// or not at all.
func (cache *Trickplay) Save(ctx context.Context, fileID string, stored trickplay.Stored) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("save trickplay manifest: %w", err)
	}

	if !isFileID(fileID) {
		return fmt.Errorf("trickplay %q: %w", fileID, domain.ErrInvalid)
	}

	content, err := json.Marshal(storedManifest{
		TileWidth: stored.Manifest.TileWidth, TileHeight: stored.Manifest.TileHeight,
		Columns: stored.Manifest.Columns, Rows: stored.Manifest.Rows,
		IntervalNS: int64(stored.Manifest.Interval), SheetCount: stored.Manifest.SheetCount, Key: stored.Key,
	})
	if err != nil {
		return fmt.Errorf("encode trickplay manifest: %w", err)
	}

	opened, err := cache.root.open(true)
	if err != nil {
		return err
	}

	defer func() { _ = opened.Close() }()

	if err := opened.MkdirAll(fileID, directoryMode); err != nil {
		return fmt.Errorf("create trickplay directory: %w", err)
	}

	temporary := fileID + pathSeparator + manifestTempName
	if err := opened.WriteFile(temporary, content, manifestMode); err != nil {
		return fmt.Errorf("write trickplay manifest: %w", err)
	}

	if err := opened.Rename(temporary, cache.manifestPath(fileID)); err != nil {
		return fmt.Errorf("publish trickplay manifest: %w", err)
	}

	return nil
}

// Touch marks the entry as used now: it sets the modification time of the manifest, which is
// what Entries reports as the last use.
func (cache *Trickplay) Touch(ctx context.Context, fileID string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("touch trickplay entry: %w", err)
	}

	if !isFileID(fileID) {
		return fmt.Errorf("trickplay %q: %w", fileID, domain.ErrInvalid)
	}

	opened, err := cache.root.open(false)
	if err != nil {
		return err
	}

	defer func() { _ = opened.Close() }()

	now := time.Now()
	if err := opened.Chtimes(cache.manifestPath(fileID), now, now); err != nil {
		return fmt.Errorf("touch trickplay entry: %w", err)
	}

	return nil
}

// Entries lists the cached file directories with their size and last use.
func (cache *Trickplay) Entries(ctx context.Context) ([]trickplay.Entry, error) {
	opened, err := cache.root.open(false)
	if isAbsent(err) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	defer func() { _ = opened.Close() }()

	directories, err := fs.ReadDir(opened.FS(), ".")
	if err != nil {
		return nil, fmt.Errorf("list trickplay cache: %w", err)
	}

	var entries []trickplay.Entry

	for _, directory := range directories {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("list trickplay cache: %w", ctxErr)
		}

		if !directory.IsDir() || !isFileID(directory.Name()) {
			continue
		}

		entries = append(entries, entryOf(opened.FS(), directory.Name()))
	}

	return entries, nil
}

// entryOf sums the files of a directory; the last use is the manifest's modification time,
// else the directory's.
func entryOf(files fs.FS, fileID string) trickplay.Entry {
	entry := trickplay.Entry{FileID: fileID}

	_ = fs.WalkDir(files, fileID, func(_ string, item fs.DirEntry, err error) error {
		if err != nil || !item.Type().IsRegular() {
			return nil //nolint:nilerr // a vanishing file only makes the size smaller
		}

		if info, infoErr := item.Info(); infoErr == nil {
			entry.Bytes += info.Size()
		}

		return nil
	})

	entry.LastUsed = modTime(files, fileID+pathSeparator+manifestName)
	if entry.LastUsed.IsZero() {
		entry.LastUsed = modTime(files, fileID)
	}

	return entry
}

func modTime(files fs.FS, name string) time.Time {
	info, err := fs.Stat(files, name)
	if err != nil {
		return time.Time{}
	}

	return info.ModTime()
}

// Remove deletes the directory of a file ID with its sheets and manifest. A missing directory
// is no error.
func (cache *Trickplay) Remove(ctx context.Context, fileID string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("remove trickplay entry: %w", err)
	}

	if !isFileID(fileID) {
		return fmt.Errorf("trickplay %q: %w", fileID, domain.ErrInvalid)
	}

	opened, err := cache.root.open(false)
	if isAbsent(err) {
		return nil
	}

	if err != nil {
		return err
	}

	defer func() { _ = opened.Close() }()

	if err := opened.RemoveAll(fileID); err != nil {
		return fmt.Errorf("remove trickplay entry %s: %w", fileID, err)
	}

	return nil
}

// SweepStaging deletes the staging directories a hard kill left in the file directories.
func (cache *Trickplay) SweepStaging(ctx context.Context) error {
	opened, err := cache.root.open(false)
	if isAbsent(err) {
		return nil
	}

	if err != nil {
		return err
	}

	defer func() { _ = opened.Close() }()

	directories, err := fs.ReadDir(opened.FS(), ".")
	if err != nil {
		return fmt.Errorf("sweep trickplay staging: %w", err)
	}

	for _, directory := range directories {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("sweep trickplay staging: %w", ctxErr)
		}

		if directory.IsDir() && isFileID(directory.Name()) {
			if err := removeStaging(opened, directory.Name()); err != nil {
				return err
			}
		}
	}

	return nil
}

func removeStaging(opened *os.Root, fileID string) error {
	children, err := fs.ReadDir(opened.FS(), fileID)
	if err != nil {
		return fmt.Errorf("sweep trickplay staging: %w", err)
	}

	for _, child := range children {
		if !child.IsDir() || !strings.HasPrefix(child.Name(), stagingPrefix) {
			continue
		}

		if err := opened.RemoveAll(fileID + pathSeparator + child.Name()); err != nil {
			return fmt.Errorf("remove staging directory: %w", err)
		}
	}

	return nil
}

var (
	_ trickplay.Store = (*Trickplay)(nil)
	_ trickplay.Cache = (*Trickplay)(nil)
)
