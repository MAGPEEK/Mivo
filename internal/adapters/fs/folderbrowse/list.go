package folderbrowse

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// List returns the readable sub-directories of relativePath below the root ("" is the root).
// Links that leave the root, junctions, hidden and system folders and reserved directories
// are left out; folders that cannot be read are only counted.
func (b *Browser) List(ctx context.Context, rootID, relativePath string) (Listing, error) {
	root, err := b.rootByID(rootID)
	if err != nil {
		return Listing{}, err
	}

	native, err := nativePath(relativePath)
	if err != nil {
		return Listing{}, err
	}

	// The gate keeps calls on a hung mount from piling up abandoned goroutines.
	if err := b.gate(ctx, root); err != nil {
		return Listing{}, err
	}

	return bounded(ctx, b.timeout, func(callCtx context.Context) (Listing, error) {
		return b.list(callCtx, root, relativePath, native)
	})
}

func (b *Browser) list(ctx context.Context, root rootEntry, relativePath, native string) (Listing, error) {
	opened, err := openRoot(root.path)
	if err != nil {
		return Listing{}, err
	}

	defer func() { _ = opened.Close() }()

	if b.isReservedResolved(filepath.Join(root.path, native)) {
		return Listing{}, fmt.Errorf("folder %q is reserved: %w", relativePath, domain.ErrInvalid)
	}

	if hiddenSegment(opened, native) {
		return Listing{}, fmt.Errorf("folder %q is hidden: %w", relativePath, domain.ErrInvalid)
	}

	target := directoryTarget{opened: opened, root: root, relative: relativePath, native: native}

	candidates, err := b.candidates(ctx, target)
	if err != nil {
		return Listing{}, err
	}

	return b.filterReadable(ctx, opened, candidates)
}

// directoryTarget is the directory being listed inside its opened root.
type directoryTarget struct {
	opened   *os.Root
	root     rootEntry
	relative string
	native   string
}

// candidate is a visible sub-directory that still needs its readability check.
type candidate struct {
	name     string
	relative string
	native   string
}

func (b *Browser) candidates(ctx context.Context, target directoryTarget) ([]candidate, error) {
	directory, err := target.opened.Open(target.native)
	if err != nil {
		return nil, classify(target.relative, err)
	}

	defer func() { _ = directory.Close() }()

	var found []candidate

	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("list %q: %w", target.relative, err)
		}

		entries, readErr := directory.ReadDir(readBatch)
		found = append(found, b.visibleCandidates(target, entries)...)

		if errors.Is(readErr, io.EOF) {
			break
		}

		if readErr != nil {
			return nil, classify(target.relative, readErr)
		}
	}

	sortCandidates(found)

	return found, nil
}

func (b *Browser) visibleCandidates(target directoryTarget, entries []fs.DirEntry) []candidate {
	var found []candidate

	for _, entry := range entries {
		childNative := entry.Name()
		if target.native != rootDirectory {
			childNative = filepath.Join(target.native, entry.Name())
		}

		if b.visibleDirectory(target, entry, childNative) {
			found = append(found, candidate{
				name: entry.Name(), relative: joinRelative(target.relative, entry.Name()), native: childNative,
			})
		}
	}

	return found
}

// visibleDirectory applies the listing rules to one entry. Links are resolved through the
// root, which refuses every target outside it; junctions show up as irregular entries.
func (b *Browser) visibleDirectory(target directoryTarget, entry fs.DirEntry, childNative string) bool {
	name := entry.Name()
	if library.IsHiddenFolderName(name) || entry.Type()&fs.ModeIrregular != 0 || hiddenByAttribute(entry) {
		return false
	}

	if b.isReserved(filepath.Join(target.root.path, childNative)) {
		return false
	}

	if entry.IsDir() {
		return true
	}

	if entry.Type()&fs.ModeSymlink == 0 {
		return false
	}

	info, err := target.opened.Stat(childNative)
	if err != nil || !info.IsDir() {
		return false
	}

	// A link into a reserved directory of the same root must stay hidden too.
	return !b.isReservedResolved(filepath.Join(target.root.path, childNative))
}

func sortCandidates(found []candidate) {
	slices.SortFunc(found, func(left, right candidate) int {
		if byFold := strings.Compare(strings.ToLower(left.name), strings.ToLower(right.name)); byFold != 0 {
			return byFold
		}

		return strings.Compare(left.name, right.name)
	})
}

// filterReadable checks candidates in sorted order with bounded concurrency and stops as soon
// as one more than the cap is known to be readable.
func (b *Browser) filterReadable(ctx context.Context, opened *os.Root, candidates []candidate) (Listing, error) {
	var listing Listing

	for start := 0; start < len(candidates); start += readabilityConcurrency {
		if err := ctx.Err(); err != nil {
			return Listing{}, fmt.Errorf("check folders: %w", err)
		}

		chunk := candidates[start:min(start+readabilityConcurrency, len(candidates))]
		readable := make([]bool, len(chunk))

		var group sync.WaitGroup

		for index, entry := range chunk {
			group.Go(func() { readable[index] = isReadable(opened, entry.native) })
		}

		group.Wait()

		for index, entry := range chunk {
			switch {
			case !readable[index]:
				listing.UnreadableCount++
			case len(listing.Folders) >= b.capacity:
				listing.Truncated = true

				return listing, nil
			default:
				listing.Folders = append(listing.Folders, Folder{Name: entry.name, Path: entry.relative})
			}
		}
	}

	return listing, nil
}

// isReadable opens the directory and reads one entry, which is what a scan needs first.
func isReadable(opened *os.Root, native string) bool {
	directory, err := opened.Open(native)
	if err != nil {
		return false
	}

	defer func() { _ = directory.Close() }()

	_, err = directory.ReadDir(1)

	return err == nil || errors.Is(err, io.EOF)
}

// classify maps a file system error to a domain error without leaking absolute paths: only
// the underlying error (without the path of a PathError) is wrapped. Everything that is not
// missing, forbidden or a transient I/O error (os.Root's escape error, loops, wrong types)
// counts as an invalid path.
func classify(relativePath string, err error) error {
	inner := withoutPath(err)

	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("folder %q: %w", relativePath, domain.ErrNotFound)
	case errors.Is(err, fs.ErrPermission):
		return fmt.Errorf("folder %q: %w", relativePath, domain.ErrForbidden)
	case isTransient(err):
		return fmt.Errorf("folder %q: %w: %w", relativePath, domain.ErrUnavailable, inner)
	default:
		return fmt.Errorf("folder %q: %w: %w", relativePath, domain.ErrInvalid, inner)
	}
}

// withoutPath returns the error inside a PathError, which carries the absolute path.
func withoutPath(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}

	return err
}

func isTransient(err error) bool {
	for _, transient := range transientErrors() {
		if errors.Is(err, transient) {
			return true
		}
	}

	return false
}
