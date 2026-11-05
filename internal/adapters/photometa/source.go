package photometa

import (
	"errors"
	"fmt"
	"io"
)

// source is a bounded view of a file: every read is checked against size before it reaches
// the reader, so offsets taken from the file can never read outside it.
type source struct {
	reader io.ReaderAt
	size   int64
}

// readAt fills buf from offset or fails; offsets outside the source are ErrCorrupt.
func (src source) readAt(buf []byte, offset int64) error {
	if offset < 0 || int64(len(buf)) > src.size-offset {
		return fmt.Errorf("read %d bytes at %d of %d: %w", len(buf), offset, src.size, ErrCorrupt)
	}

	read, err := src.reader.ReadAt(buf, offset)
	if read == len(buf) {
		return nil
	}

	if err == nil || errors.Is(err, io.EOF) {
		err = io.ErrUnexpectedEOF
	}

	return fmt.Errorf("read %d bytes at %d: %w", len(buf), offset, err)
}

// section returns the sub-range [offset, offset+length) as its own source.
func (src source) section(offset, length int64) (source, error) {
	if offset < 0 || length < 0 || length > src.size-offset {
		return source{}, fmt.Errorf("section %d+%d of %d: %w", offset, length, src.size, ErrCorrupt)
	}

	return source{reader: io.NewSectionReader(src.reader, offset, length), size: length}, nil
}

// readBytes reads length bytes at offset, refusing lengths above limit before allocating.
func (src source) readBytes(offset, length, limit int64) ([]byte, error) {
	if length < 0 || length > limit {
		return nil, fmt.Errorf("read %d bytes, limit %d: %w", length, limit, ErrCorrupt)
	}

	buf := make([]byte, length)
	if err := src.readAt(buf, offset); err != nil {
		return nil, err
	}

	return buf, nil
}
