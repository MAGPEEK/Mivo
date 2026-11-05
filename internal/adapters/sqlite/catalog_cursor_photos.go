package sqlite

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
)

// Fields of a photo cursor, which leads with photoCursorTag so it is never mistaken for the
// cursor of another listing, followed by the album, the group of the last child (sub-album
// or photo), its ID, its capture time (empty for an album) and its text sort key last, the
// only field that may hold arbitrary bytes.
const (
	photoCursorTag    = "photos"
	photoCursorFields = 6
	albumGroup        = "album"
	photoGroup        = "photo"
)

// photoPosition is the place of a child in album order: sub-albums by (sortTitle, id), then
// photos by (takenAt, nameKey, id). The zero position lies before every child.
type photoPosition struct {
	photo     bool
	id        string
	sortTitle string
	takenAt   int64
	nameKey   string
}

// encodePhotoCursor returns the opaque cursor of the page after position, as base64url without
// padding of the photoCursorFields joined by cursorSeparator.
func encodePhotoCursor(albumID string, position photoPosition) string {
	fields := []string{photoCursorTag, albumID, albumGroup, position.id, "", position.sortTitle}
	if position.photo {
		fields = []string{
			photoCursorTag, albumID, photoGroup, position.id, strconv.FormatInt(position.takenAt, numberBase),
			position.nameKey,
		}
	}

	return base64.RawURLEncoding.EncodeToString([]byte(strings.Join(fields, cursorSeparator)))
}

// decodePhotoCursor returns the zero position for an empty cursor. A cursor that was not
// issued for this album wraps domain.ErrInvalid.
func decodePhotoCursor(cursor, albumID string) (photoPosition, error) {
	if cursor == "" {
		return photoPosition{}, nil
	}

	raw, err := decodeCursorBytes(cursor)
	if err != nil {
		return photoPosition{}, err
	}

	fields := strings.SplitN(string(raw), cursorSeparator, photoCursorFields)
	if len(fields) != photoCursorFields || fields[0] != photoCursorTag || fields[1] != albumID || fields[3] == "" {
		return photoPosition{}, fmt.Errorf("cursor was not issued for this album: %w", domain.ErrInvalid)
	}

	switch fields[2] {
	case albumGroup:
		if fields[4] != "" {
			return photoPosition{}, fmt.Errorf("album cursor with a capture time: %w", domain.ErrInvalid)
		}

		return photoPosition{id: fields[3], sortTitle: fields[5]}, nil
	case photoGroup:
		takenAt, err := strconv.ParseInt(fields[4], numberBase, numberBitSize)
		// The round trip refuses "+5" and "05", so one position has one cursor.
		if err != nil || strconv.FormatInt(takenAt, numberBase) != fields[4] || fields[5] == "" {
			return photoPosition{}, fmt.Errorf("cursor has no photo position: %w", domain.ErrInvalid)
		}

		return photoPosition{photo: true, id: fields[3], takenAt: takenAt, nameKey: fields[5]}, nil
	default:
		return photoPosition{}, fmt.Errorf("cursor has an unknown group: %w", domain.ErrInvalid)
	}
}
