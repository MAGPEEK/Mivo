package sqlite

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// Fields of a track cursor, which leads with trackCursorTag so it is never mistaken for the
// cursor of another listing. noTrackNumber stands for a track without number.
const (
	trackCursorTag    = "tracks"
	trackCursorFields = 6
	noTrackNumber     = -1
)

// encodeTrackCursor returns the opaque cursor of the page after position: base64url without
// padding of "tracks NUL album NUL disc NUL track NUL id NUL file name".
func encodeTrackCursor(albumID string, position trackPosition) string {
	track := noTrackNumber
	if number, ok := position.order.Track.Get(); ok {
		track = number
	}

	fields := []string{
		trackCursorTag, albumID, strconv.Itoa(position.order.Disc), strconv.Itoa(track), position.id,
		position.order.FileName,
	}

	return base64.RawURLEncoding.EncodeToString([]byte(strings.Join(fields, cursorSeparator)))
}

// decodeTrackCursor reports no position for an empty cursor. A cursor that was not issued
// for this album wraps domain.ErrInvalid.
func decodeTrackCursor(cursor, albumID string) (position trackPosition, found bool, err error) {
	if cursor == "" {
		return trackPosition{}, false, nil
	}

	raw, err := decodeCursorBytes(cursor)
	if err != nil {
		return trackPosition{}, false, err
	}

	fields := strings.SplitN(string(raw), cursorSeparator, trackCursorFields)
	if len(fields) != trackCursorFields || fields[0] != trackCursorTag || fields[1] != albumID || fields[4] == "" {
		return trackPosition{}, false, fmt.Errorf("cursor was not issued for these tracks: %w", domain.ErrInvalid)
	}

	disc, discErr := strconv.Atoi(fields[2])
	track, trackErr := strconv.Atoi(fields[3])

	if discErr != nil || trackErr != nil || disc < firstDisc || track < noTrackNumber {
		return trackPosition{}, false, fmt.Errorf("cursor has no track position: %w", domain.ErrInvalid)
	}

	order := library.TrackOrder{Disc: disc, FileName: fields[5]}
	if track != noTrackNumber {
		order.Track = library.NumberOf(track)
	}

	return trackPosition{order: order, id: fields[4]}, true, nil
}
