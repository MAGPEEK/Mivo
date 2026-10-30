package sqlite

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
)

// cursorSeparator splits the item ID from the sort title inside a cursor. Neither an ID nor
// a title derived from a file name can contain a NUL byte, and unlike JSON the raw bytes keep
// a sort title that is not valid UTF-8 (a Linux file name) exactly.
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

	raw, err := base64.RawURLEncoding.Strict().DecodeString(cursor)
	if err != nil {
		return itemKey{}, fmt.Errorf("cursor is not base64url: %w", domain.ErrInvalid)
	}

	id, sortTitle, found := strings.Cut(string(raw), cursorSeparator)
	if !found || id == "" {
		return itemKey{}, fmt.Errorf("cursor has no item key: %w", domain.ErrInvalid)
	}

	return itemKey{sortTitle: sortTitle, id: id}, nil
}
