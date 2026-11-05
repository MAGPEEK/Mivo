// Package photometa reads the EXIF metadata of photos and extracts the embedded JPEG
// preview of RAW files in pure Go (ADR 0029): TIFF IFDs inside JPEG (APP1), TIFF-based RAW
// (CR2, NEF, ARW, DNG, ORF, RW2), ISOBMFF Exif items (HEIC, HEIF, AVIF), Canon CR3 and
// Fujifilm RAF. Olympus ORF is read for metadata; its full-size preview sits in the maker
// note, so only an IFD1 thumbnail is found as its preview.
//
// A broken Exif or GPS IFD pointer is reported as ErrCorrupt rather than ignored, so a
// damaged file is visible instead of silently losing its capture time.
//
// The files are hostile input: every read is bounded by the file size and a size constant,
// IFD loops are detected, iteration counts are capped and the context is checked in every
// loop, so a crafted file can neither panic nor allocate or loop without bound. The caller
// passes an opened file or an already validated path; this package does not resolve paths
// against library roots.
package photometa

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/photo"
)

// ErrUnsupported reports a file whose format or layout this package cannot read. It wraps
// domain.ErrUnsupported.
var ErrUnsupported = fmt.Errorf("photometa: unsupported format: %w", domain.ErrUnsupported)

// ErrCorrupt reports metadata whose structure is broken (offsets outside the file, loops,
// impossible sizes). It wraps domain.ErrInvalid.
var ErrCorrupt = fmt.Errorf("photometa: corrupt metadata: %w", domain.ErrInvalid)

// ErrNoPreview reports a file without a usable embedded JPEG preview.
var ErrNoPreview = errors.New("photometa: no embedded preview")

// Reader reads photo metadata and previews from files on disk. The zero value is ready.
type Reader struct{}

// NewReader returns a Reader.
func NewReader() *Reader {
	return &Reader{}
}

// ReadInfo opens the file at path (already validated against the library roots by the
// caller) and returns its EXIF metadata, see ReadInfoFrom.
func (reader *Reader) ReadInfo(ctx context.Context, path string) (photo.Info, error) {
	var info photo.Info

	err := withFile(path, func(file io.ReaderAt, size int64) error {
		var readErr error
		info, readErr = ReadInfoFrom(ctx, file, size)

		return readErr
	})
	if err != nil {
		return photo.Info{}, fmt.Errorf("read photo info %s: %w", path, err)
	}

	return info, nil
}

// ExtractPreview opens the file at path (already validated by the caller) and returns its
// largest embedded JPEG preview, see ExtractPreviewFrom.
func (reader *Reader) ExtractPreview(ctx context.Context, path string) ([]byte, error) {
	var preview []byte

	err := withFile(path, func(file io.ReaderAt, size int64) error {
		var extractErr error
		preview, extractErr = ExtractPreviewFrom(ctx, file, size)

		return extractErr
	})
	if err != nil {
		return nil, fmt.Errorf("extract photo preview %s: %w", path, err)
	}

	return preview, nil
}

// ReadInfoFrom returns the EXIF metadata of the size bytes in file. Missing tags are zero
// values and a file without EXIF yields a zero Info; broken structures return ErrCorrupt
// and unknown formats ErrUnsupported. TakenAt is the EXIF wall-clock reading as UTC
// (photo.Info). The result always passes photo.Info.Validate.
func ReadInfoFrom(ctx context.Context, file io.ReaderAt, size int64) (photo.Info, error) {
	src := source{reader: file, size: size}

	kind, err := detectFormat(src)
	if err != nil {
		return photo.Info{}, err
	}

	switch kind {
	case formatJPEG:
		return readJPEGInfo(ctx, src)
	case formatTIFF:
		return readTIFFFileInfo(ctx, src)
	case formatRAF:
		return readRAFInfo(ctx, src)
	case formatHEIF:
		return readHEIFInfo(ctx, src)
	case formatCR3:
		return readCR3Info(ctx, src)
	case formatUnknown:
	}

	return photo.Info{}, ErrUnsupported
}

// ExtractPreviewFrom returns the largest renderable (baseline or progressive) JPEG embedded
// in the size bytes of file: the IFD0/IFD1/SubIFD JPEGInterchangeFormat, JPEG strips and the
// RW2 JpgFromRaw for TIFF-based files and EXIF blocks, the RAF header JPEG or the CR3 PRVW
// box. For a JPEG file that is its EXIF thumbnail. Previews above maxPreviewBytes are
// skipped; without a candidate the error is ErrNoPreview.
func ExtractPreviewFrom(ctx context.Context, file io.ReaderAt, size int64) ([]byte, error) {
	src := source{reader: file, size: size}

	kind, err := detectFormat(src)
	if err != nil {
		return nil, err
	}

	switch kind {
	case formatJPEG:
		return extractJPEGPreview(ctx, src)
	case formatTIFF:
		return extractTIFFFilePreview(ctx, src)
	case formatRAF:
		return extractRAFPreview(ctx, src)
	case formatHEIF:
		return extractHEIFPreview(ctx, src)
	case formatCR3:
		return extractCR3Preview(ctx, src)
	case formatUnknown:
	}

	return nil, ErrUnsupported
}

// withFile opens path read-only and hands it to use with its size.
func withFile(path string, use func(file io.ReaderAt, size int64) error) error {
	file, err := os.Open(path) //nolint:gosec // the caller validated path against the library roots
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	// A read-only file has nothing to flush, so a close error carries no information.
	defer func() { _ = file.Close() }()

	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat: %w", err)
	}

	return use(file, stat.Size())
}
