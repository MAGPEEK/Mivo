package disc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"

	"github.com/magpeek/mivo/internal/domain/media"
)

// Bounds for hostile discs. Real discs stay far below them: a Blu-ray holds at most a few
// thousand playlists and clips, an MPLS file rarely exceeds a few hundred KiB.
const (
	// maxDirEntries caps one directory listing.
	maxDirEntries = 8192
	// dirBatchSize is how many entries are listed per ReadDir call while checking the cap.
	dirBatchSize = 512
	// maxReadBudget caps the bytes read from all playlists and IFO files of one disc.
	maxReadBudget = 128 << 20
)

// errTooLarge marks a file above its size limit; such a file is skipped like an unparsable one.
var errTooLarge = errors.New("disc file too large")

// fileTree reads a disc tree within the limits above and checks cancellation before each I/O.
type fileTree struct {
	fsys fs.FS
	// ctx is stored because fs.FS methods carry no context; it lives only for one resolution.
	ctx    context.Context //nolint:containedctx // see above
	budget int64
}

func newFileTree(ctx context.Context, fsys fs.FS) *fileTree {
	return &fileTree{fsys: fsys, ctx: ctx, budget: maxReadBudget}
}

// list returns the entries of dir sorted by name, or ErrDiscUnsupported above maxDirEntries.
func (files *fileTree) list(dir string) ([]fs.DirEntry, error) {
	if err := files.ctx.Err(); err != nil {
		return nil, fmt.Errorf("list %s: %w", dir, err)
	}
	handle, err := files.fsys.Open(dir)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", dir, err)
	}
	defer func() { _ = handle.Close() }() // read-only; a close error loses nothing

	reader, ok := handle.(fs.ReadDirFile)
	if !ok {
		return nil, fmt.Errorf("list %s: not a directory: %w", dir, fs.ErrInvalid)
	}
	var entries []fs.DirEntry
	for {
		batch, err := reader.ReadDir(dirBatchSize)
		entries = append(entries, batch...)
		if len(entries) > maxDirEntries {
			return nil, fmt.Errorf("list %s: more than %d entries: %w", dir, maxDirEntries, media.ErrDiscUnsupported)
		}
		if errors.Is(err, io.EOF) || (err == nil && len(batch) == 0) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", dir, err)
		}
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })

	return entries, nil
}

// read returns the file content up to limit bytes (errTooLarge above it) and charges the
// per-disc budget; an exhausted budget is ErrDiscUnsupported.
func (files *fileTree) read(name string, limit int64) ([]byte, error) {
	if err := files.ctx.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	allowed := min(limit, files.budget)
	handle, err := files.fsys.Open(name)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	defer func() { _ = handle.Close() }() // read-only; a close error loses nothing

	data, err := io.ReadAll(io.LimitReader(handle, allowed+1))
	files.budget -= int64(len(data))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	if int64(len(data)) > allowed {
		if allowed < limit {
			return nil, fmt.Errorf("read %s: read budget exhausted: %w", name, media.ErrDiscUnsupported)
		}
		return nil, fmt.Errorf("read %s: %w", name, errTooLarge)
	}

	return data, nil
}

// size returns the size of a regular file (following links inside the tree).
func (files *fileTree) size(name string) (int64, error) {
	if err := files.ctx.Err(); err != nil {
		return 0, fmt.Errorf("stat %s: %w", name, err)
	}
	info, err := fs.Stat(files.fsys, name)
	if err != nil {
		return 0, fmt.Errorf("stat %s: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return 0, fmt.Errorf("stat %s: not a regular file: %w", name, fs.ErrInvalid)
	}

	return info.Size(), nil
}

// discFolder returns the folder that holds the disc structure: the child named folderName
// (any case) when the tree root is its parent, else the root itself.
func (files *fileTree) discFolder(folderName string) (string, error) {
	child, found, err := files.childDir(".", folderName)
	if err != nil || !found {
		return ".", err
	}

	return child, nil
}

// childDir finds the sub-directory of dir whose lower-cased name is lowerName.
func (files *fileTree) childDir(dir, lowerName string) (string, bool, error) {
	entries, err := files.list(dir)
	if err != nil {
		return "", false, err
	}
	for _, entry := range entries {
		if entry.IsDir() && lowerASCII(entry.Name()) == lowerName {
			return joinPath(dir, entry.Name()), true, nil
		}
	}

	return "", false, nil
}

// joinPath joins a tree directory and an entry name with a slash ("." is the root).
func joinPath(dir, name string) string {
	if dir == "." {
		return name
	}

	return dir + "/" + name
}

// lowerASCII lower-cases ASCII letters only. Unicode case folding is avoided on purpose: it
// would let names such as "ſtream" (long s) match the fixed disc names.
func lowerASCII(name string) string {
	lowered := []byte(name)
	for index, char := range lowered {
		if 'A' <= char && char <= 'Z' {
			lowered[index] = char + ('a' - 'A')
		}
	}

	return string(lowered)
}

// isSkippable reports whether a file read error only disqualifies that file (missing, too
// large, unreadable entry) rather than the whole disc (cancellation, exhausted budget).
func isSkippable(err error) bool {
	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) &&
		!errors.Is(err, media.ErrDiscUnsupported)
}
