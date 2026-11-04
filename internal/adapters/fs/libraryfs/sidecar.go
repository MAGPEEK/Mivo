package libraryfs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/magpeek/mivo/internal/domain"
)

// sidecarChunkBytes is the read size between two context checks; sidecars are small (NFO
// text, artwork), so a few chunks read a whole file.
const sidecarChunkBytes = 64 * 1024

// ReadSidecar returns the bytes of a sidecar file (NFO or image) below the library root. The
// file is opened through os.Root and checked to be regular on the open handle, like Open.
// A file larger than maxBytes is refused with domain.ErrUnsupported before it is read; a file
// that grows past maxBytes while it is read is refused as well, so the read stays bounded.
// ctx is checked before opening and between chunks.
//
// Errors: those of Open (domain.ErrForbidden for a path leaving the root, domain.ErrNotFound
// for a vanished or non-regular file, domain.ErrUnavailable for failing storage), a
// non-positive maxBytes domain.ErrInvalid, and ctx.Err() when ctx ends.
func (fileSystem *FileSystem) ReadSidecar(ctx context.Context, root, relativePath string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, fmt.Errorf("read sidecar %q: limit %d: %w", relativePath, maxBytes, domain.ErrInvalid)
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("read sidecar %q: %w", relativePath, err)
	}

	file, err := fileSystem.Open(root, relativePath)
	if err != nil {
		return nil, fmt.Errorf("read sidecar: %w", err)
	}

	defer closeFile(file)

	content, err := readBounded(ctx, file, maxBytes)
	if err != nil {
		return nil, fmt.Errorf("read sidecar %q: %w", relativePath, err)
	}

	return content, nil
}

// readBounded reads at most maxBytes from an open regular file, checking ctx between chunks.
func readBounded(ctx context.Context, file *os.File, maxBytes int64) ([]byte, error) {
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat: %w", classifyRootError(err))
	}

	if info.Size() > maxBytes {
		return nil, errTooLarge(maxBytes)
	}

	// One byte beyond the limit tells a file that grew from one that fits exactly.
	limited := io.LimitReader(file, maxBytes+1)

	var content bytes.Buffer

	content.Grow(int(info.Size()))

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		_, err := io.CopyN(&content, limited, sidecarChunkBytes)
		if int64(content.Len()) > maxBytes {
			return nil, errTooLarge(maxBytes)
		}

		if errors.Is(err, io.EOF) {
			return content.Bytes(), nil
		}

		if err != nil {
			return nil, fmt.Errorf("read: %w", classifyRootError(err))
		}
	}
}

func errTooLarge(maxBytes int64) error {
	return fmt.Errorf("sidecar larger than %d bytes: %w", maxBytes, domain.ErrUnsupported)
}
