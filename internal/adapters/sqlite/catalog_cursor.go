package sqlite

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
)

// cursorSeparator splits the fields of a cursor. Neither an ID nor a title derived from a file
// name can contain a NUL byte, and unlike JSON the raw bytes keep a sort title that is not
// valid UTF-8 (a Linux file name) exactly.
const cursorSeparator = "\x00"

// itemKey is the position of an item in the keyset order (sort_title, id). The zero key lies
// before every item.
type itemKey struct {
	sortTitle string
	id        string
}

// encodeCursor returns the opaque cursor of the page after key: base64url without padding
// of "<id> NUL <sort title>".
func encodeCursor(key itemKey) string {
	return base64.RawURLEncoding.EncodeToString([]byte(key.id + cursorSeparator + key.sortTitle))
}

// decodeCursor returns the zero key for an empty cursor. A cursor that encodeCursor cannot
// have produced wraps domain.ErrInvalid.
func decodeCursor(cursor string) (itemKey, error) {
	if cursor == "" {
		return itemKey{}, nil
	}

	raw, err := decodeCursorBytes(cursor)
	if err != nil {
		return itemKey{}, err
	}

	id, sortTitle, found := strings.Cut(string(raw), cursorSeparator)
	if !found || id == "" || id == childCursorTag {
		return itemKey{}, fmt.Errorf("cursor has no item key: %w", domain.ErrInvalid)
	}

	return itemKey{sortTitle: sortTitle, id: id}, nil
}

// decodeCursorBytes decodes the canonical base64url text of a cursor. Go's decoder skips
// carriage returns and line feeds even in strict mode, which would give one key several
// cursors, so they are refused first.
func decodeCursorBytes(cursor string) ([]byte, error) {
	if strings.ContainsAny(cursor, "\r\n") {
		return nil, fmt.Errorf("cursor contains a line break: %w", domain.ErrInvalid)
	}

	raw, err := base64.RawURLEncoding.Strict().DecodeString(cursor)
	if err != nil {
		return nil, fmt.Errorf("cursor is not base64url: %w", domain.ErrInvalid)
	}

	return raw, nil
}

// Fields of a child cursor, which leads with childCursorTag so the cursors of the two kinds of
// listing can never be mistaken for each other.
const (
	childCursorTag      = "children"
	childCursorFields   = 7
	ascendingDirection  = "asc"
	descendingDirection = "desc"
)

// Number parsing of the cursor fields.
const (
	numberBase    = 10
	numberBitSize = 64
)

// The order groups of child order: numbered children first, then the specials (season 0)
// and the unnumbered ones.
const (
	numberedChildGroup = 0
	trailingChildGroup = 1
)

// childKey is the position of a child in child order (see queries/catalog_read.sql): the
// order group (0 numbered, 1 specials and unnumbered), the number, the sort title and the ID.
type childKey struct {
	group     int64
	number    int64
	sortTitle string
	id        string
}

// childCursorScope is what a child cursor is bound to: the item whose children are paged and
// the direction, because a key of one order means nothing in the reversed order.
type childCursorScope struct {
	parentID   string
	descending bool
}

func (scope childCursorScope) direction() string {
	if scope.descending {
		return descendingDirection
	}

	return ascendingDirection
}

// encodeChildCursor returns the opaque cursor of the page after key.
func encodeChildCursor(scope childCursorScope, key childKey) string {
	fields := []string{
		childCursorTag, scope.direction(), scope.parentID,
		strconv.FormatInt(key.group, numberBase), strconv.FormatInt(key.number, numberBase),
		key.id, key.sortTitle,
	}

	return base64.RawURLEncoding.EncodeToString([]byte(strings.Join(fields, cursorSeparator)))
}

// decodeChildCursor reports no key for an empty cursor. A cursor that was not issued for this
// item and direction wraps domain.ErrInvalid.
func decodeChildCursor(cursor string, scope childCursorScope) (key childKey, found bool, err error) {
	if cursor == "" {
		return childKey{}, false, nil
	}

	raw, err := decodeCursorBytes(cursor)
	if err != nil {
		return childKey{}, false, err
	}

	fields := strings.SplitN(string(raw), cursorSeparator, childCursorFields)
	if len(fields) != childCursorFields || fields[0] != childCursorTag ||
		fields[1] != scope.direction() || fields[2] != scope.parentID || fields[5] == "" {
		return childKey{}, false, fmt.Errorf("cursor was not issued for these children: %w", domain.ErrInvalid)
	}

	group, groupErr := strconv.ParseInt(fields[3], numberBase, numberBitSize)
	number, numberErr := strconv.ParseInt(fields[4], numberBase, numberBitSize)

	if groupErr != nil || numberErr != nil || group < numberedChildGroup || group > trailingChildGroup {
		return childKey{}, false, fmt.Errorf("cursor has no child key: %w", domain.ErrInvalid)
	}

	return childKey{group: group, number: number, id: fields[5], sortTitle: fields[6]}, true, nil
}
