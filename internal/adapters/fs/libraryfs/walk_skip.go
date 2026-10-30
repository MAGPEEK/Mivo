package libraryfs

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// unreadable handles an entry or directory that could not be read. A lost root ends the walk
// with domain.ErrUnavailable, a vanished entry is logged at debug, anything else is logged and
// recorded as skipped so that the caller keeps its catalog entries.
func (w *walk) unreadable(relativePath string, err error) error {
	if rootErr := w.requireReadableRoot(); rootErr != nil {
		return rootErr
	}

	if errors.Is(err, fs.ErrNotExist) {
		w.logger.Debug("skipping entry that disappeared during the scan", pathAttr(relativePath))
		return nil
	}

	w.skip(relativePath, err)

	return nil
}

// unresolvedLink handles a link that os.Root would not follow. A link the server may not read
// is unreadable; every other refusal (target outside the root, missing target, loop) is the
// expected result of confinement and logged at info.
func (w *walk) unresolvedLink(relativePath string, err error) error {
	if rootErr := w.requireReadableRoot(); rootErr != nil {
		return rootErr
	}

	if errors.Is(err, fs.ErrPermission) {
		w.skip(relativePath, err)
		return nil
	}

	w.logger.Info("skipping link that leaves the library root or cannot be followed",
		pathAttr(relativePath), errorAttr(err))

	return nil
}

func (w *walk) skip(relativePath string, err error) {
	w.logger.Warn("skipping unreadable entry", pathAttr(relativePath), errorAttr(err))
	w.skipped = append(w.skipped, relativePath)
}

// requireReadableRoot tells a failing entry from a failing root. Without it, a NAS share that
// drops mid-walk would look like a library whose remaining folders are empty, and the scan
// would delete their catalog entries.
func (w *walk) requireReadableRoot() error {
	directory, err := w.root.OpenFile(rootDirectory, openDirectoryFlags, 0)
	if err == nil {
		_, err = directory.ReadDir(probeEntries)
		closeFile(directory)
	}

	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("library root became unreadable: %w: %w", domain.ErrUnavailable, err)
	}

	return nil
}

// isSkippedName reports hidden entries and system folders. Checking the name before the type
// is safe: no video file is named like a system folder.
func isSkippedName(name string) bool {
	return library.IsHiddenEntry(name) || library.IsIgnoredDirectory(name)
}

// isPlainNonVideoFile lets the walk skip most files without any I/O.
func isPlainNonVideoFile(entry fs.DirEntry) bool {
	return entry.Type().IsRegular() && !library.IsVideoFileName(entry.Name())
}

func pathAttr(relativePath string) slog.Attr {
	return slog.String(pathLogKey, relativePath)
}

// errorAttr logs an error without the path of a *fs.PathError, which may be absolute (errors of
// a file opened through the root carry the root's path).
func errorAttr(err error) slog.Attr {
	var pathError *fs.PathError
	if errors.As(err, &pathError) {
		return slog.String(errorLogKey, pathError.Op+": "+pathError.Err.Error())
	}

	return slog.String(errorLogKey, err.Error())
}
