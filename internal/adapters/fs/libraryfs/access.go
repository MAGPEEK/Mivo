package libraryfs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/magpeek/mivo/internal/domain"
)

// Resolve returns the absolute path of a regular file below the library root for a process
// that cannot take an open handle (FFmpeg). The file must be reachable through os.Root, and
// its path with all symlinks of root and file evaluated must lie inside the evaluated root
// (compared case-insensitively on Windows). The small window between this check and the
// process opening the path is accepted (ADR 0022).
//
// Errors: an unsafe relative path, a file outside the root or a missing permission wraps
// domain.ErrForbidden, a missing file, a directory or another non-regular file
// domain.ErrNotFound, a root that cannot be opened or an I/O error of a dropped disk or share
// domain.ErrUnavailable, a relative root domain.ErrInvalid.
func (*FileSystem) Resolve(root, relativePath string) (string, error) {
	native, err := checkedRelativePath(relativePath)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", relativePath, err)
	}

	if err := requireRegularFileInRoot(root, native); err != nil {
		return "", fmt.Errorf("resolve %q: %w", relativePath, err)
	}

	resolved, err := evaluateInsideRoot(root, native)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", relativePath, err)
	}

	return resolved, nil
}

// Open opens a regular file below the library root for reading through os.Root, for the
// direct-play handler. Errors are those of Resolve. The caller closes the file.
func (*FileSystem) Open(root, relativePath string) (*os.File, error) {
	native, err := checkedRelativePath(relativePath)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", relativePath, err)
	}

	libraryRoot, err := openLibraryRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", relativePath, err)
	}

	defer closeRoot(libraryRoot)

	file, err := openRegularFile(libraryRoot, native)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", relativePath, err)
	}

	return file, nil
}

// checkedRelativePath applies the catalog and host OS path rules. A path that breaks them was
// never written by a scan, so it is treated as an attempt to leave the root.
func checkedRelativePath(relativePath string) (string, error) {
	native, err := nativeRelativePath(relativePath)
	if err != nil {
		// Only the message of err is kept: it wraps domain.ErrInvalid, and the caller must see
		// one unambiguous sentinel.
		return "", fmt.Errorf("unsafe relative path: %w (%s)", domain.ErrForbidden, err.Error())
	}

	return native, nil
}

func requireRegularFileInRoot(root, native string) error {
	libraryRoot, err := openLibraryRoot(root)
	if err != nil {
		return err
	}

	defer closeRoot(libraryRoot)

	info, err := libraryRoot.Stat(native)
	if err != nil {
		return classifyRootError(err)
	}

	return requireRegular(info)
}

// openRegularFile opens without blocking (openFileFlags), so a FIFO cannot hang the caller,
// and checks the type on the open handle, which no later rename can change.
func openRegularFile(libraryRoot *os.Root, native string) (*os.File, error) {
	file, err := libraryRoot.OpenFile(native, openFileFlags, 0)
	if err != nil {
		return nil, classifyRootError(err)
	}

	opened, err := file.Stat()
	if err == nil {
		err = requireRegular(opened)
	}

	if err != nil {
		closeFile(file)
		return nil, fmt.Errorf("check opened file: %w", err)
	}

	return file, nil
}

func requireRegular(info fs.FileInfo) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file: %w", domain.ErrNotFound)
	}

	return nil
}

// classifyRootError maps an os.Root error: a missing entry (or a file used as a directory) is
// domain.ErrNotFound, an I/O error of a dropped disk or share domain.ErrUnavailable. Anything
// else, above all a missing permission or a link that leaves the root, is domain.ErrForbidden.
// os.Root does not export its "path escapes" error, so the escape cannot be told apart from
// other refusals, and refusing is the safe answer.
func classifyRootError(err error) error {
	switch {
	// First: Windows also reports a lost network path as fs.ErrNotExist.
	case isTransientError(err):
		return fmt.Errorf("library storage failed: %w: %w", domain.ErrUnavailable, err)
	case errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR):
		return fmt.Errorf("%w: %w", domain.ErrNotFound, err)
	default:
		return fmt.Errorf("not reachable inside the library root: %w: %w", domain.ErrForbidden, err)
	}
}

func isTransientError(err error) bool {
	return slices.ContainsFunc(transientErrors(), func(transient error) bool { return errors.Is(err, transient) })
}

// evaluateInsideRoot evaluates the symlinks of root and file and verifies containment. On
// Windows filepath.EvalSymlinks does not evaluate junctions; os.Root has refused them already.
func evaluateInsideRoot(root, native string) (string, error) {
	evaluatedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("evaluate library root %s: %w: %w", root, domain.ErrUnavailable, err)
	}

	evaluated, err := filepath.EvalSymlinks(filepath.Join(evaluatedRoot, native))
	if err != nil {
		return "", fmt.Errorf("evaluate file: %w", classifyRootError(err))
	}

	if !isWithin(evaluatedRoot, evaluated, isWindows, filepath.Separator) {
		return "", fmt.Errorf("evaluated path leaves the library root: %w", domain.ErrForbidden)
	}

	return evaluated, nil
}
