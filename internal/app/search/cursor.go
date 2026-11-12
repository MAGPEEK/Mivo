package search

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// A search cursor is base64url (no padding) of "search NUL <type> NUL <offset>": the rank of
// the next hit, bound to the pivot so it cannot page another one. Ranks are stable between
// pages as long as the catalog does not change; a change may repeat or skip a hit, which a
// results page tolerates.
const (
	cursorTag       = "search"
	cursorSeparator = "\x00"
	cursorFields    = 3
)

func encodeCursor(searchType Type, offset int) string {
	text := strings.Join([]string{cursorTag, string(searchType), strconv.Itoa(offset)}, cursorSeparator)

	return base64.RawURLEncoding.EncodeToString([]byte(text))
}

// decodeCursor returns 0 for an empty cursor. A cursor not issued for this type, or one
// pointing outside the MaxLimit hits the index ranks, wraps ErrInvalidCursor.
func decodeCursor(cursor string, searchType Type) (int, error) {
	if cursor == "" {
		return 0, nil
	}

	raw, err := base64.RawURLEncoding.Strict().DecodeString(cursor)
	if err != nil || strings.ContainsAny(cursor, "\r\n") {
		return 0, fmt.Errorf("not base64url: %w", ErrInvalidCursor)
	}

	fields := strings.Split(string(raw), cursorSeparator)
	if len(fields) != cursorFields || fields[0] != cursorTag || fields[1] != string(searchType) {
		return 0, fmt.Errorf("not issued for type %q: %w", searchType, ErrInvalidCursor)
	}

	offset, err := strconv.Atoi(fields[2])
	if err != nil || offset <= 0 || offset >= MaxLimit || strconv.Itoa(offset) != fields[2] {
		return 0, fmt.Errorf("no position: %w", ErrInvalidCursor)
	}

	return offset, nil
}
