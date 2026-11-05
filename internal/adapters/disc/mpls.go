package disc

import (
	"encoding/binary"
	"fmt"
	"time"
)

// MPLS layout (Blu-ray "Movie PlayList" file, as read by libbluray's mpls_parse). All
// offsets are in bytes; play item offsets are relative to the item's length field.
const (
	mplsMagic = "MPLS"
	// mplsPlaylistStartOffset holds the absolute offset of the PlayList structure.
	mplsPlaylistStartOffset = 8
	// playlistItemCountOffset follows the PlayList length (4) and 2 reserved bytes.
	playlistItemCountOffset = 6
	// playlistFirstItemOffset follows the item count and the sub-path count (2 each).
	playlistFirstItemOffset = 10

	itemLengthFieldSize = 2
	itemClipNameOffset  = 2
	clipIDLength        = 5
	itemCodecOffset     = 7
	codecIDLength       = 4
	itemInTimeOffset    = 14
	itemOutTimeOffset   = 18
	// minItemLength is the item length (excluding its length field) that covers OUT_time.
	minItemLength = itemOutTimeOffset + 4 - itemLengthFieldSize

	// mplsTicksPerSecond is the 45 kHz clock of IN_time/OUT_time.
	mplsTicksPerSecond = 45000
	// maxPlaylistSize skips implausibly large playlist files instead of reading them.
	maxPlaylistSize = 1 << 20
)

// playItem is one clip reference of a playlist (angle 1 for multi-angle items).
type playItem struct {
	clipID  string
	in, out uint32
}

// playlist is the ordered list of play items of one MPLS file.
type playlist []playItem

// duration sums the in/out spans. Each span is below 2^32 ticks (about 26 hours) and an MPLS
// file holds at most 65535 items, so the sum cannot overflow.
func (list playlist) duration() time.Duration {
	var total time.Duration
	for _, entry := range list {
		total += ticksToDuration(entry.out - entry.in)
	}

	return total
}

func ticksToDuration(ticks uint32) time.Duration {
	return time.Duration(ticks) * time.Second / mplsTicksPerSecond
}

// repeatsClip reports a playlist that references a clip more than once: menu loops and
// decoy playlists do that, a main feature does not.
func (list playlist) repeatsClip() bool {
	seen := make(map[string]struct{}, len(list))
	for _, entry := range list {
		if _, ok := seen[entry.clipID]; ok {
			return true
		}
		seen[entry.clipID] = struct{}{}
	}

	return false
}

// parsePlaylist parses the play items of an MPLS file. Every offset and the item count are
// checked against the data, so hostile input fails with errMalformed instead of panicking;
// the loop ends after at most 65535 items.
func parsePlaylist(data []byte) (playlist, error) {
	view := byteView(data)
	magic, err := view.span(0, int64(len(mplsMagic)))
	if err != nil || string(magic) != mplsMagic {
		return nil, fmt.Errorf("%w: not an MPLS file", errMalformed)
	}
	start, err := view.u32(mplsPlaylistStartOffset)
	if err != nil {
		return nil, malformed("playlist start", err)
	}
	count, err := view.u16(int64(start) + playlistItemCountOffset)
	if err != nil {
		return nil, malformed("play item count", err)
	}

	list := make(playlist, 0, min(int(count), len(data)/minItemLength))
	offset := int64(start) + playlistFirstItemOffset
	for index := range int(count) {
		entry, next, err := parsePlayItem(view, offset)
		if err != nil {
			return nil, fmt.Errorf("play item %d: %w", index, err)
		}
		list = append(list, entry)
		offset = next
	}

	return list, nil
}

// parsePlayItem reads the item at offset and returns it with the offset of the next item.
func parsePlayItem(view byteView, offset int64) (playItem, int64, error) {
	length, err := view.u16(offset)
	if err != nil {
		return playItem{}, 0, malformed("item length", err)
	}
	if length < minItemLength {
		return playItem{}, 0, fmt.Errorf("%w: item length %d", errMalformed, length)
	}
	item, err := view.span(offset, itemLengthFieldSize+int64(length))
	if err != nil {
		return playItem{}, 0, malformed("item body", err)
	}

	// All fields below lie inside item because length >= minItemLength.
	clipID := string(item[itemClipNameOffset : itemClipNameOffset+clipIDLength])
	codec := string(item[itemCodecOffset : itemCodecOffset+codecIDLength])
	if !isClipID(clipID) || (codec != "M2TS" && codec != "FMTS") {
		return playItem{}, 0, fmt.Errorf("%w: clip %q codec %q", errMalformed, clipID, codec)
	}
	in := binary.BigEndian.Uint32(item[itemInTimeOffset:])
	out := binary.BigEndian.Uint32(item[itemOutTimeOffset:])
	if out < in {
		return playItem{}, 0, fmt.Errorf("%w: OUT_time %d before IN_time %d", errMalformed, out, in)
	}

	return playItem{clipID: clipID, in: in, out: out}, offset + int64(len(item)), nil
}

// isClipID reports a five-digit clip id; only such ids are ever turned into file names.
func isClipID(id string) bool {
	if len(id) != clipIDLength {
		return false
	}
	for index := range len(id) {
		if id[index] < '0' || id[index] > '9' {
			return false
		}
	}

	return true
}
