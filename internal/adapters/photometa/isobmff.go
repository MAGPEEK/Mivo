package photometa

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
)

const (
	boxHeaderSize      = 8
	largeSizeFieldSize = 8
	uuidSize           = 16
	fullBoxHeaderSize  = 4
	// boxSizeToEnd and boxSizeLarge are the special 32-bit box sizes (ISO 14496-12 4.2).
	boxSizeToEnd = 0
	boxSizeLarge = 1
	// maxBoxesPerLevel bounds the boxes walked in one container.
	maxBoxesPerLevel = 4096
	boxTypeUUID      = "uuid"
)

// box is one ISOBMFF box; the payload excludes the header and the uuid user type.
type box struct {
	kind         string
	uuid         string
	payloadStart int64
	payloadEnd   int64
}

// span is the byte range [start, end) of a container.
type span struct {
	start int64
	end   int64
}

// boxQuery selects a box by type and, when uuid is not empty, by its uuid user type.
type boxQuery struct {
	kind string
	uuid string
}

// payload is the range of the box contents.
func (current box) payload() span {
	return span{start: current.payloadStart, end: current.payloadEnd}
}

// whole is the range of the entire source.
func (src source) whole() span {
	return span{start: 0, end: src.size}
}

// walkBoxes visits the boxes within src; visit returns true to stop.
func walkBoxes(ctx context.Context, src source, within span, visit func(box) bool) error {
	position, end := within.start, within.end
	for range maxBoxesPerLevel {
		if position >= end {
			return nil
		}

		if err := ctx.Err(); err != nil {
			return fmt.Errorf("walk boxes: %w", err)
		}

		current, err := readBox(src, position, end)
		if err != nil {
			return err
		}

		if visit(current) {
			return nil
		}

		position = current.payloadEnd
	}

	return fmt.Errorf("more than %d boxes: %w", maxBoxesPerLevel, ErrCorrupt)
}

// readBox reads the box header at position inside a container ending at end.
func readBox(src source, position, end int64) (box, error) {
	header := make([]byte, boxHeaderSize+largeSizeFieldSize)
	if err := src.readAt(header[:boxHeaderSize], position); err != nil {
		return box{}, fmt.Errorf("read box header: %w", err)
	}

	size := int64(binary.BigEndian.Uint32(header))
	current := box{kind: string(header[4:boxHeaderSize]), payloadStart: position + boxHeaderSize}

	switch size {
	case boxSizeToEnd:
		size = end - position
	case boxSizeLarge:
		if err := src.readAt(header[boxHeaderSize:], position+boxHeaderSize); err != nil {
			return box{}, fmt.Errorf("read large box size: %w", err)
		}

		large := binary.BigEndian.Uint64(header[boxHeaderSize:])
		if large > math.MaxInt64 {
			return box{}, fmt.Errorf("box size %d: %w", large, ErrCorrupt)
		}

		size = int64(large)
		current.payloadStart += largeSizeFieldSize
	}

	if size < current.payloadStart-position || size > end-position {
		return box{}, fmt.Errorf("box %q of %d bytes at %d: %w", current.kind, size, position, ErrCorrupt)
	}

	current.payloadEnd = position + size

	if current.kind == boxTypeUUID {
		if err := readUUID(src, &current); err != nil {
			return box{}, err
		}
	}

	return current, nil
}

func readUUID(src source, current *box) error {
	if current.payloadEnd-current.payloadStart < uuidSize {
		return fmt.Errorf("uuid box too small: %w", ErrCorrupt)
	}

	userType := make([]byte, uuidSize)
	if err := src.readAt(userType, current.payloadStart); err != nil {
		return fmt.Errorf("read uuid: %w", err)
	}

	current.uuid = string(userType)
	current.payloadStart += uuidSize

	return nil
}

// findBox returns the first box within src that matches query.
func findBox(ctx context.Context, src source, within span, query boxQuery) (box, bool, error) {
	var found box

	matched := false
	err := walkBoxes(ctx, src, within, func(current box) bool {
		matched = current.kind == query.kind && (query.uuid == "" || current.uuid == query.uuid)
		if matched {
			found = current
		}

		return matched
	})

	return found, matched && err == nil, err
}
