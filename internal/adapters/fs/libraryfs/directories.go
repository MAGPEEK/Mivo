package libraryfs

import (
	"context"
	"fmt"
	"io/fs"
	"os"

	"github.com/magpeek/mivo/internal/domain"
)

// ListDirectories returns the names of the sub-directories of a directory below the library
// root, for the folder view (task M5-11). directory is a catalog path ("/"-separated, empty
// for the root itself). Everything goes through os.Root, so neither the directory nor an entry
// can be reached through a symlink or junction that leaves the root; the evaluated path of the
// directory is checked against the evaluated root as well. Hidden entries and NAS or OS system
// folders are left out, like the scan does, and so are entries whose path the catalog cannot
// store. A link to a directory inside the root is listed.
//
// Errors: an unsafe path, a directory outside the root or a missing permission wrap
// domain.ErrForbidden; a missing directory or a file wraps domain.ErrNotFound; an unreachable
// root domain.ErrUnavailable; a done ctx its error.
func (*FileSystem) ListDirectories(ctx context.Context, root, directory string) ([]string, error) {
	native := rootDirectory

	if directory != "" {
		var err error
		if native, err = checkedRelativePath(directory); err != nil {
			return nil, fmt.Errorf("list directories of %q: %w", directory, err)
		}
	}

	libraryRoot, err := openLibraryRoot(root)
	if err != nil {
		return nil, fmt.Errorf("list directories of %q: %w", directory, err)
	}

	defer closeRoot(libraryRoot)

	entries, err := readDirectory(libraryRoot, root, native)
	if err != nil {
		return nil, fmt.Errorf("list directories of %q: %w", directory, err)
	}

	names, err := directoryNames(ctx, libraryRoot, directory, entries)
	if err != nil {
		return nil, fmt.Errorf("list directories of %q: %w", directory, err)
	}

	return names, nil
}

// readDirectory opens the directory through os.Root, checks that it is one and lies inside
// the evaluated root, and reads its entries.
func readDirectory(libraryRoot *os.Root, root, native string) ([]fs.DirEntry, error) {
	opened, err := libraryRoot.OpenFile(native, openDirectoryFlags, 0)
	if err != nil {
		return nil, classifyRootError(err)
	}

	defer closeFile(opened)

	info, err := opened.Stat()
	if err != nil {
		return nil, classifyRootError(err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %w", domain.ErrNotFound)
	}

	if native != rootDirectory {
		if _, err := evaluateInsideRoot(root, native); err != nil {
			return nil, err
		}
	}

	entries, err := opened.ReadDir(-1)
	if err != nil {
		return nil, classifyRootError(err)
	}

	return entries, nil
}

// directoryNames keeps the entries that are directories, or links that os.Root resolves to a
// directory inside the root. directory is the catalog path of the listed directory.
func directoryNames(ctx context.Context, libraryRoot *os.Root, directory string, entries []fs.DirEntry) ([]string, error) {
	parent := directory
	if parent == "" {
		parent = rootDirectory
	}

	var names []string

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("list directories: %w", err)
		}

		name := entry.Name()
		if isSkippedName(name) {
			continue
		}

		child, err := nativeRelativePath(joinRelative(parent, name))
		if err != nil {
			continue
		}

		if entry.IsDir() || (entry.Type()&linkModes != 0 && isLinkedDirectory(libraryRoot, child)) {
			names = append(names, name)
		}
	}

	return names, nil
}

// isLinkedDirectory reports whether a link resolves, inside the root, to a directory.
// os.Root refuses links that leave the root, so those report false.
func isLinkedDirectory(libraryRoot *os.Root, native string) bool {
	info, err := libraryRoot.Stat(native)

	return err == nil && info.IsDir()
}
