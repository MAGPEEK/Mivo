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
	trackCursorFields = 7
	noTrackNumber     = -1
)

// encodeTrackCursor returns the opaque cursor of the page after position: base64url without
// padding of "tracks NUL direction NUL album NUL disc NUL track NUL id NUL file name". Like a
// child cursor it is bound to the album and the direction.
func encodeTrackCursor(scope childCursorScope, position trackPosition) string {
	track := noTrackNumber
	if number, ok := position.order.Track.Get(); ok {
		track = number
	}

	fields := []string{
		trackCursorTag, scope.direction(), scope.parentID, strconv.Itoa(position.order.Disc), strconv.Itoa(track),
		position.id, position.order.FileName,
	}

	return base64.RawURLEncoding.EncodeToString([]byte(strings.Join(fields, cursorSeparator)))
}

// childrenCursor is the position a children cursor names: a child key for the children of a
// show, season or artist, a track position for the tracks of an album. Which one is valid
// depends on the parent's kind, which is known only after reading it.
type childrenCursor struct {
	child    childKey
	hasChild bool
	track    trackPosition
	hasTrack bool
}

// decodeChildrenCursor accepts a child cursor or a track cursor issued for this item and
// direction. Anything else wraps domain.ErrInvalid.
func decodeChildrenCursor(cursor string, scope childCursorScope) (childrenCursor, error) {
	child, hasChild, childErr := decodeChildCursor(cursor, scope)
	if childErr == nil {
		return childrenCursor{child: child, hasChild: hasChild}, nil
	}

	track, hasTrack, trackErr := decodeTrackCursor(cursor, scope)
	if trackErr == nil {
		return childrenCursor{track: track, hasTrack: hasTrack}, nil
	}

	return childrenCursor{}, childErr
}

// decodeTrackCursor reports no position for an empty cursor. A cursor that was not issued
// for this album and direction wraps domain.ErrInvalid.
func decodeTrackCursor(cursor string, scope childCursorScope) (position trackPosition, found bool, err error) {
	if cursor == "" {
		return trackPosition{}, false, nil
	}

	raw, err := decodeCursorBytes(cursor)
	if err != nil {
		return trackPosition{}, false, err
	}

	fields := strings.SplitN(string(raw), cursorSeparator, trackCursorFields)
	if len(fields) != trackCursorFields || fields[0] != trackCursorTag || fields[1] != scope.direction() ||
		fields[2] != scope.parentID || fields[5] == "" {
		return trackPosition{}, false, fmt.Errorf("cursor was not issued for these tracks: %w", domain.ErrInvalid)
	}

	disc, discErr := strconv.Atoi(fields[3])
	track, trackErr := strconv.Atoi(fields[4])

	if discErr != nil || trackErr != nil || disc < firstDisc || track < noTrackNumber {
		return trackPosition{}, false, fmt.Errorf("cursor has no track position: %w", domain.ErrInvalid)
	}

	order := library.TrackOrder{Disc: disc, FileName: fields[6]}
	if track != noTrackNumber {
		order.Track = library.NumberOf(track)
	}

	return trackPosition{order: order, id: fields[5]}, true, nil
}
