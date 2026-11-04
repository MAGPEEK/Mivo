// Package iso reads the file tree of a DVD/Blu-ray image (UDF with ISO 9660 bridge fallback)
// without mounting it and without cgo. Files open as io.ReaderAt; every offset taken from the
// image is bounds-checked, directory sizes and extent counts are capped (ADR 0029).
//
// Each directory is parsed once and cached (bounded), directory cycles along a path and paths
// deeper than 64 components are rejected. A hostile image can still contain a directed acyclic
// graph of directories with exponentially many distinct paths, so callers that walk the whole
// tree (fs.WalkDir) must bound the total work themselves (entry count or time).
package iso

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// maxPathDepth is the deepest path (in components) lookups accept.
const maxPathDepth = 64

// entry is one node of the tree. UDF entries are resolved lazily (size and extents need the
// file entry); ISO 9660 entries are complete when listed.
type entry struct {
	name     string
	dir      bool
	size     int64
	extents  []Extent
	resolved bool
	icb      udfICB
	id       nodeID // identity of a directory; see nodeID
}

// volume is implemented by the UDF and ISO 9660 readers.
type volume interface {
	root() entry
	list(ctx context.Context, dir entry) ([]entry, error)
	resolve(ctx context.Context, item *entry) error
}

var (
	errNotUDF = errors.New("no UDF anchor")
	errNotISO = errors.New("no ISO 9660 descriptor")
)

// Image is a read-only file tree over a disc image. It implements fs.FS, fs.ReadDirFS and
// fs.StatFS; opened files also implement io.ReaderAt and io.Seeker.
type Image struct {
	src   *source
	vol   volume
	cache *listCache
	// ctx is stored because fs.FS methods carry no context; WithContext swaps it per request.
	ctx context.Context //nolint:containedctx // see above
}

// Open parses the volume structures of the image of the given size. UDF is preferred; if it is
// absent or unusable the ISO 9660 tree is used. ctx also governs later reads (see WithContext).
func Open(ctx context.Context, reader io.ReaderAt, size int64) (*Image, error) {
	src := &source{reader: reader, size: size}
	vol, udfErr := openUDF(ctx, src)
	if udfErr == nil {
		return &Image{src: src, vol: vol, cache: newListCache(), ctx: ctx}, nil
	}
	if ctx.Err() != nil {
		return nil, fmt.Errorf("open disc image: %w", ctx.Err())
	}
	vol, isoErr := openISO9660(ctx, src)
	if isoErr == nil {
		return &Image{src: src, vol: vol, cache: newListCache(), ctx: ctx}, nil
	}
	if errors.Is(udfErr, errNotUDF) && errors.Is(isoErr, errNotISO) {
		return nil, corrupt("no UDF or ISO 9660 file system found")
	}
	return nil, fmt.Errorf("open disc image: %w", errors.Join(udfErr, isoErr))
}

// WithContext returns a view whose reads are cancelled by ctx.
func (img *Image) WithContext(ctx context.Context) *Image {
	view := *img
	view.ctx = ctx
	return &view
}

func (img *Image) lookup(name string) (entry, error) {
	if !fs.ValidPath(name) {
		return entry{}, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	current := img.vol.root()
	if name == "." {
		return current, nil
	}
	parts := strings.Split(name, "/")
	if len(parts) > maxPathDepth { // bounds walks over directory cycles in hostile images
		return entry{}, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	onPath := map[nodeID]struct{}{current.id: {}}
	for _, part := range parts {
		if !current.dir {
			return entry{}, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		}
		children, err := img.list(img.ctx, current)
		if err != nil {
			return entry{}, &fs.PathError{Op: "open", Path: name, Err: err}
		}
		next, found := findChild(children, part)
		if !found {
			return entry{}, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		}
		if next.dir {
			if _, repeated := onPath[next.id]; repeated {
				return entry{}, &fs.PathError{Op: "open", Path: name, Err: corrupt("directory cycle at %q", part)}
			}
			onPath[next.id] = struct{}{}
		}
		current = next
	}
	return current, nil
}

// list returns the cached listing of dir, parsing it on first use. The result is shared and
// must not be modified.
func (img *Image) list(ctx context.Context, dir entry) ([]entry, error) {
	if cached, ok := img.cache.get(dir.id); ok {
		return cached, nil
	}
	listing, err := img.vol.list(ctx, dir)
	if err != nil {
		return nil, err
	}
	img.cache.put(dir.id, listing)
	return listing, nil
}

// findChild matches exactly first, then case-insensitively because ISO 9660 names are upper case.
func findChild(children []entry, name string) (entry, bool) {
	for _, child := range children {
		if child.name == name {
			return child, true
		}
	}
	var match entry
	matches := 0
	for _, child := range children {
		if strings.EqualFold(child.name, name) {
			match = child
			matches++
		}
	}
	return match, matches == 1 // ambiguous case-insensitive matches count as not found
}

func (img *Image) resolved(name string) (entry, error) {
	item, err := img.lookup(name)
	if err != nil {
		return entry{}, err
	}
	if err := img.vol.resolve(img.ctx, &item); err != nil {
		return entry{}, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	return item, nil
}

// Extents returns the image byte ranges that hold the file, in order, for use as DiscClip offsets.
func (img *Image) Extents(name string) ([]Extent, error) {
	item, err := img.resolved(name)
	if err != nil {
		return nil, err
	}
	if item.dir {
		return nil, &fs.PathError{Op: "extents", Path: name, Err: fs.ErrInvalid}
	}
	return append([]Extent(nil), item.extents...), nil
}

// Open implements fs.FS.
func (img *Image) Open(name string) (fs.File, error) {
	item, err := img.resolved(name)
	if err != nil {
		return nil, err
	}
	if name == "." {
		item.name = "."
	} else {
		item.name = path.Base(name)
	}
	if !item.dir {
		return &file{img: img, item: item}, nil
	}
	children, err := img.children(name, item)
	if err != nil {
		return nil, err
	}
	return &dirFile{item: item, children: children}, nil
}

func (img *Image) children(name string, dir entry) ([]fs.DirEntry, error) {
	shared, err := img.list(img.ctx, dir)
	if err != nil {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: err}
	}
	list := append([]entry(nil), shared...) // the cached slice must stay unsorted and unshared
	sort.Slice(list, func(i, j int) bool { return list[i].name < list[j].name })
	out := make([]fs.DirEntry, len(list))
	for i, child := range list {
		out[i] = dirEntry{img: img, item: child}
	}
	return out, nil
}

// ReadDir implements fs.ReadDirFS.
func (img *Image) ReadDir(name string) ([]fs.DirEntry, error) {
	item, err := img.lookup(name)
	if err != nil {
		return nil, err
	}
	if !item.dir {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: errors.New("not a directory")}
	}
	return img.children(name, item)
}

// Stat implements fs.StatFS.
func (img *Image) Stat(name string) (fs.FileInfo, error) {
	item, err := img.resolved(name)
	if err != nil {
		return nil, err
	}
	if name == "." {
		item.name = "."
	} else {
		item.name = path.Base(name)
	}
	return fileInfo{item: item}, nil
}
