package sqlite

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
)

func TestPhotoCursorRoundTrip(t *testing.T) {
	t.Parallel()

	positions := []photoPosition{
		{id: "album-1", sortTitle: "beach"},
		{id: "album-2", sortTitle: ""},
		{photo: true, id: "photo-1", takenAt: -5, nameKey: "a\x01b.jpg"},
		{photo: true, id: "photo-2", takenAt: 1_620_129_600_000, nameKey: "img.jpg"},
	}

	for _, position := range positions {
		got, err := decodePhotoCursor(encodePhotoCursor("album", position), "album")
		if err != nil || got != position {
			t.Errorf("round trip of %+v = %+v, %v", position, got, err)
		}
	}
}

func TestDecodePhotoCursorRejectsForeignCursors(t *testing.T) {
	t.Parallel()

	raw := func(fields ...string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(strings.Join(fields, cursorSeparator)))
	}

	cases := map[string]string{
		"not base64":         "%%%",
		"other tag":          raw("tracks", "album", "photo", "id", "1", "a"),
		"other album":        raw(photoCursorTag, "other", "photo", "id", "1", "a"),
		"too few fields":     raw(photoCursorTag, "album", "photo", "id", "1"),
		"empty id":           raw(photoCursorTag, "album", "photo", "", "1", "a"),
		"unknown group":      raw(photoCursorTag, "album", "video", "id", "1", "a"),
		"album with time":    raw(photoCursorTag, "album", albumGroup, "id", "1", "a"),
		"photo without time": raw(photoCursorTag, "album", photoGroup, "id", "", "a"),
		"signed time":        raw(photoCursorTag, "album", photoGroup, "id", "+1", "a"),
		"padded time":        raw(photoCursorTag, "album", photoGroup, "id", "01", "a"),
		"photo without name": raw(photoCursorTag, "album", photoGroup, "id", "1", ""),
		"item cursor":        encodeCursor(itemKey{sortTitle: "a", id: "id"}),
	}

	for name, cursor := range cases {
		if _, err := decodePhotoCursor(cursor, "album"); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: error = %v, want ErrInvalid", name, err)
		}
	}
}
