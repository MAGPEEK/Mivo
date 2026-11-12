// Package cachedir implements the disk caches below the cache directory: the converted
// WebVTT subtitles (<cache>/subtitles/<fileID>/) and the trickplay sprite sheets
// (<cache>/trickplay/<fileID>/). Every operation is confined to its own subdirectory through
// os.Root, and the directory names are catalog file IDs in UUID form, so no request text and no
// symlink can lead a read, write or delete elsewhere (ADR 0030 sections 3 and 8).
package cachedir

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"uuid"

	"github.com/magpeek/mivo/internal/domain"
)

const (
	// subtitlesDirectoryName and trickplayDirectoryName are the folders below the cache
	// directory; the services derive the same names for their destinations.
	subtitlesDirectoryName = "subtitles"
	trickplayDirectoryName = "trickplay"
	// directoryMode is the permission of the directories this package creates.
	directoryMode = 0o750
	// pathSeparator separates the elements of the slash paths of os.Root.
	pathSeparator = "/"
)

// isFileID reports a canonical lowercase UUID, the shape of every catalog file ID. Only such a
// name is ever used as a directory name below a cache root.
func isFileID(text string) bool {
	parsed, err := uuid.Parse(text)

	return err == nil && parsed.String() == text
}

// confinedRoot is an absolute cache root; every path the caches accept must lie below it.
type confinedRoot struct {
	path string
}

func newConfinedRoot(cacheDir, name string) (confinedRoot, error) {
	if !filepath.IsAbs(cacheDir) {
		return confinedRoot{}, fmt.Errorf("cache directory %q is not absolute: %w", cacheDir, domain.ErrInvalid)
	}

	return confinedRoot{path: filepath.Join(cacheDir, name)}, nil
}

// open opens the root, creating the directory first when create is set. A root that does not
// exist yet wraps fs.ErrNotExist without create.
func (root confinedRoot) open(create bool) (*os.Root, error) {
	if create {
		if err := os.MkdirAll(root.path, directoryMode); err != nil {
			return nil, fmt.Errorf("create cache directory: %w", err)
		}
	}

	opened, err := os.OpenRoot(root.path)
	if err != nil {
		return nil, fmt.Errorf("open cache directory: %w", err)
	}

	return opened, nil
}

// relative returns the slash-separated path of an absolute path below the root, which must
// have exactly depth elements, the first one a file ID. Anything else wraps domain.ErrForbidden.
func (root confinedRoot) relative(path string, depth int) (string, error) {
	relative, err := filepath.Rel(root.path, path)
	if err != nil || !filepath.IsLocal(relative) {
		return "", fmt.Errorf("path %q is outside the cache: %w", path, domain.ErrForbidden)
	}

	slashed := filepath.ToSlash(relative)

	parts := strings.Split(slashed, pathSeparator)
	if len(parts) != depth || !isFileID(parts[0]) {
		return "", fmt.Errorf("path %q is no cache entry: %w", path, domain.ErrForbidden)
	}

	return slashed, nil
}

// isAbsent reports a missing file or directory.
func isAbsent(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}
