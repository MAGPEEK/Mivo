package iso

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/magpeek/mivo/internal/domain"
)

const (
	sectorSize = 2048
	// maxDirBytes bounds the memory a single directory listing may use.
	maxDirBytes = 1 << 20
	// maxDirEntries bounds the entries parsed from one directory.
	maxDirEntries = 1 << 14
	// maxExtents bounds the extents of one file; reads scan them linearly, so keep it modest.
	maxExtents = 1 << 12
)

// ErrInvalidImage reports a corrupt, truncated or unsupported disc image.
var ErrInvalidImage = fmt.Errorf("disc image: %w", domain.ErrInvalid)

func corrupt(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidImage, fmt.Sprintf(format, args...))
}

// Extent is a contiguous byte range inside the image. DiscClip.Offset/Length use the same meaning.
type Extent struct {
	Offset, Length int64
}

// source wraps the image with bounds checks so hostile descriptors cannot read outside it.
type source struct {
	reader io.ReaderAt
	size   int64
}

func (src *source) readAt(ctx context.Context, buf []byte, offset int64) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("read image: %w", err)
	}
	if offset < 0 || int64(len(buf)) > src.size-offset {
		return corrupt("read %d bytes at %d beyond image size %d", len(buf), offset, src.size)
	}
	count, err := src.reader.ReadAt(buf, offset)
	if count == len(buf) {
		return nil // io.ReaderAt may return EOF together with a full read
	}
	if err == nil || errors.Is(err, io.EOF) {
		err = io.ErrUnexpectedEOF
	}
	return fmt.Errorf("read image at %d (%d of %d bytes): %w", offset, count, len(buf), err)
}

// rejectDuplicates fails on repeated names, which make lookups ambiguous in hostile images.
func rejectDuplicates(entries []entry) error {
	seen := make(map[string]struct{}, len(entries))
	for _, item := range entries {
		if _, dup := seen[item.name]; dup {
			return corrupt("duplicate directory entry %q", item.name)
		}
		seen[item.name] = struct{}{}
	}
	return nil
}

func (src *source) sector(ctx context.Context, sector int64) ([]byte, error) {
	buf := make([]byte, sectorSize)
	if err := src.readAt(ctx, buf, sector*sectorSize); err != nil {
		return nil, err
	}
	return buf, nil
}

// checkExtent rejects extents that leave the image.
func (src *source) checkExtent(extent Extent) error {
	if extent.Offset < 0 || extent.Length < 0 || extent.Length > src.size-extent.Offset {
		return corrupt("extent %d+%d beyond image size %d", extent.Offset, extent.Length, src.size)
	}
	return nil
}

// readExtents fills buf from the logical byte offset of a file made of extents.
func (src *source) readExtents(ctx context.Context, extents []Extent, buf []byte, offset int64) (int, error) {
	total := 0
	var base int64
	for _, extent := range extents {
		if len(buf) == 0 {
			break
		}
		end := base + extent.Length
		if offset < end {
			within := offset - base
			chunk := min(int64(len(buf)), extent.Length-within)
			if err := src.readAt(ctx, buf[:chunk], extent.Offset+within); err != nil {
				return total, err
			}
			total += int(chunk)
			buf = buf[chunk:]
			offset += chunk
		}
		base = end
	}
	if len(buf) > 0 {
		return total, io.EOF
	}
	return total, nil
}

// readAll loads a whole directory body, refusing sizes above maxDirBytes before allocating.
func (src *source) readAll(ctx context.Context, item entry) ([]byte, error) {
	if item.size > maxDirBytes {
		return nil, corrupt("directory %q is %d bytes", item.name, item.size)
	}
	buf := make([]byte, item.size)
	if _, err := src.readExtents(ctx, item.extents, buf, 0); err != nil {
		return nil, fmt.Errorf("read directory %q: %w", item.name, err)
	}
	return buf, nil
}
