package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

// Blu-ray (BDMV) structures, as libbluray and internal/adapters/disc read them. The writers
// fill the fields a player needs to find the playlists and their clips; they are test-corpus
// code, not a Blu-ray authoring tool.
const (
	// bdmvHeaderVersion follows the four-character type of every BDMV database file.
	bdmvHeaderVersion = "0200"
	indexMagic        = "INDX"
	movieObjectMagic  = "MOBJ"
	clipInfoMagic     = "HDMV"
	playlistMagic     = "MPLS"
	// bdmvStubSize is the size of the index, movie object and clip info stubs: the header and
	// zeroed offsets, enough for the scan to detect the folder.
	bdmvStubSize = 64

	// playlistHeaderSize is the MPLS header the PlayList structure follows; its start offset
	// is stored at playlistStartOffset.
	playlistHeaderSize  = 58
	playlistStartOffset = 8
	// playlistFixedFields is the PlayList length (4), 2 reserved bytes, the play item and the
	// sub-path counts (2 each).
	playlistFixedFields  = 10
	playlistLengthField  = 4
	playlistCountsOffset = 6
	// playItemClipCodec names an M2TS clip.
	playItemClipCodec = "M2TS"
	// playItemTrailer is the UO mask (8), random access flag, still mode (1 + 2) and an empty
	// STN table (16) after OUT_time.
	playItemTrailer = 8 + 1 + 1 + 2 + 16
	// mplsTicksPerSecond is the 45 kHz clock of IN_time and OUT_time.
	mplsTicksPerSecond = 45000
)

// BDAV source packets and the AACS stand-in flags (see markAACSUnits).
const (
	sourcePacketSize  = 192
	tpExtraHeaderSize = 4
	tsSyncByte        = 0x47
	// alignedUnitSize is the AACS encryption unit: 32 source packets.
	alignedUnitSize = 6144
	// markedAlignedUnits is how many units at the start of each clip carry the marks; the
	// encryption check reads two.
	markedAlignedUnits = 2
	// copyPermissionFlag sets the top two bits (copy_permission_indicator) of TP_extra_header.
	copyPermissionFlag = 0xC0
	// encryptedSyncByte replaces the sync byte of every packet whose header AACS would
	// encrypt (all but the first of a unit); any value but 0x47 shows the effect.
	encryptedSyncByte = 0x00
)

var errNoSourcePackets = errors.New("the clip holds no whole aligned unit of 192-byte source packets")

// playlistItem is one play item: a five-digit clip id and its length.
type playlistItem struct {
	clip   string
	length time.Duration
}

// bdmvStub is an index.bdmv, MovieObject.bdmv or clip info file: magic, version, zeros.
func bdmvStub(magic string) []byte {
	data := make([]byte, bdmvStubSize)
	copy(data, magic+bdmvHeaderVersion)

	return data
}

// moviePlaylist writes an MPLS file whose play items reference the clips in order. Each item
// runs from IN_time 0 for its length; a player plays the whole clip either way.
func moviePlaylist(items ...playlistItem) []byte {
	data := make([]byte, playlistHeaderSize)
	copy(data, playlistMagic+bdmvHeaderVersion)
	binary.BigEndian.PutUint32(data[playlistStartOffset:], playlistHeaderSize)

	list := make([]byte, playlistFixedFields)
	binary.BigEndian.PutUint16(list[playlistCountsOffset:], uint16(len(items))) //nolint:gosec // a handful of items

	for _, item := range items {
		list = append(list, playItem(item)...)
	}

	binary.BigEndian.PutUint32(list, uint32(len(list)-playlistLengthField)) //nolint:gosec // small playlists

	return append(data, list...)
}

// playItem is the length field, clip id, codec id, flags, STC id, IN_time and OUT_time,
// followed by the zeroed trailer.
func playItem(item playlistItem) []byte {
	body := make([]byte, 0, len(item.clip)+len(playItemClipCodec)+playItemTrailer)
	body = append(body, item.clip...)
	body = append(body, playItemClipCodec...)
	body = append(body, 0, 0, 0) // not multi-angle, connection condition, STC id
	body = binary.BigEndian.AppendUint32(body, 0)
	body = binary.BigEndian.AppendUint32(body, uint32(item.length*mplsTicksPerSecond/time.Second)) //nolint:gosec // seconds
	body = append(body, make([]byte, playItemTrailer)...)

	return append(binary.BigEndian.AppendUint16(nil, uint16(len(body))), body...) //nolint:gosec // fixed size
}

// markAACSUnits returns a copy of an m2ts clip whose first aligned units look like AACS
// output: copy_permission_indicator is set in every TP_extra_header, and every packet but
// the first of a unit has lost its TS sync byte, as unit encryption leaves it (AACS keeps
// only the first 16 bytes of a unit clear). These are marks, not encryption: there is no key
// and no cipher, the rest of each packet is the clear stream.
func markAACSUnits(clip []byte) ([]byte, error) {
	marked := append([]byte(nil), clip...)
	units := min(len(marked)/alignedUnitSize, markedAlignedUnits)

	if units == 0 {
		return nil, fmt.Errorf("mark AACS units of %d bytes: %w", len(marked), errNoSourcePackets)
	}

	for packet := 0; packet < units*alignedUnitSize; packet += sourcePacketSize {
		marked[packet] |= copyPermissionFlag

		if packet%alignedUnitSize != 0 {
			marked[packet+tpExtraHeaderSize] = encryptedSyncByte
		}
	}

	return marked, nil
}

// isSourcePacketStream reports whether data is a whole number of 192-byte source packets
// that each carry the TS sync byte, what the mpegts muxer writes in m2ts mode.
func isSourcePacketStream(data []byte) bool {
	if len(data) == 0 || len(data)%sourcePacketSize != 0 {
		return false
	}

	for packet := 0; packet < len(data); packet += sourcePacketSize {
		if data[packet+tpExtraHeaderSize] != tsSyncByte {
			return false
		}
	}

	return true
}
