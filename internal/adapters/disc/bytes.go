package disc

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
)

// errMalformed marks MPLS or IFO data that does not parse; the file is skipped.
var errMalformed = fmt.Errorf("malformed disc structure: %w", domain.ErrInvalid)

// errOutOfRange is returned by byteView reads beyond the data and wrapped into errMalformed.
var errOutOfRange = errors.New("offset out of range")

// byteView reads big-endian fields at offsets that come from untrusted data. Offsets are int64
// so 32-bit values from the file never overflow, also on 32-bit platforms.
type byteView []byte

// span returns length bytes at offset or errOutOfRange.
func (view byteView) span(offset, length int64) ([]byte, error) {
	if offset < 0 || length < 0 || offset > int64(len(view)) || length > int64(len(view))-offset {
		return nil, errOutOfRange
	}

	return view[offset : offset+length], nil
}

// Sizes of the big-endian integer fields.
const (
	uint16Size = 2
	uint32Size = 4
)

func (view byteView) u16(offset int64) (uint16, error) {
	field, err := view.span(offset, uint16Size)
	if err != nil {
		return 0, err
	}

	return binary.BigEndian.Uint16(field), nil
}

func (view byteView) u32(offset int64) (uint32, error) {
	field, err := view.span(offset, uint32Size)
	if err != nil {
		return 0, err
	}

	return binary.BigEndian.Uint32(field), nil
}

// malformed wraps a parse failure with its context.
func malformed(what string, err error) error {
	return fmt.Errorf("%w: %s: %w", errMalformed, what, err)
}
