//nolint:gosec // fixture writer: counts and sizes are small test constants
package disc

import (
	"encoding/binary"
	"testing/fstest"
)

// testItem describes one synthetic MPLS play item; times are in 45 kHz ticks.
type testItem struct {
	clip, codec string
	in, out     uint32
	angles      []string
}

// item returns a play item for clip with the given length in seconds.
func item(clip string, seconds uint32) testItem {
	return testItem{clip: clip, codec: "M2TS", in: 0, out: seconds * mplsTicksPerSecond}
}

const testPlaylistStart = 58

// buildMPLS writes a minimal but structurally real MPLS file (header, PlayList, PlayItems with
// an empty STN table) as libbluray's mpls_parse expects it.
func buildMPLS(items ...testItem) []byte {
	data := make([]byte, testPlaylistStart)
	copy(data, "MPLS0200")
	binary.BigEndian.PutUint32(data[8:], testPlaylistStart)

	playlist := make([]byte, 10)
	binary.BigEndian.PutUint16(playlist[6:], uint16(len(items)))
	for _, entry := range items {
		playlist = append(playlist, buildPlayItem(entry)...)
	}
	binary.BigEndian.PutUint32(playlist, uint32(len(playlist)-4))

	return append(data, playlist...)
}

func buildPlayItem(entry testItem) []byte {
	body := make([]byte, 0, 64)
	body = append(body, entry.clip...)
	body = append(body, entry.codec...)
	var flags uint16
	if len(entry.angles) > 0 {
		flags = 0x10
	}
	body = binary.BigEndian.AppendUint16(body, flags)
	body = append(body, 0) // ref_to_STC_id
	body = binary.BigEndian.AppendUint32(body, entry.in)
	body = binary.BigEndian.AppendUint32(body, entry.out)
	body = append(body, make([]byte, 8+1+1+2)...) // UO mask, random access, still mode/time
	if len(entry.angles) > 0 {
		body = append(body, byte(len(entry.angles)+1), 0)
		for _, angle := range entry.angles {
			body = append(body, angle...)
			body = append(body, "M2TS"...)
			body = append(body, 0)
		}
	}
	body = append(body, make([]byte, 16)...) // empty STN table

	return append(binary.BigEndian.AppendUint16(nil, uint16(len(body))), body...)
}

// pgcTime is a PGC playback time; rate is 1 (25 fps) or 3 (30 fps).
type pgcTime struct{ hours, minutes, seconds, frames, rate int }

// bcd encodes the time as the IFO stores it.
func (playback pgcTime) bcd() [4]byte {
	bcd := func(value int) byte { return byte(value/10<<4 | value%10) }
	return [4]byte{
		bcd(playback.hours), bcd(playback.minutes), bcd(playback.seconds),
		byte(playback.rate<<6) | bcd(playback.frames),
	}
}

const testPGCSize = 16

// buildIFO writes a VTS IFO with its PGCIT in sector 1 and one PGC per playback time.
func buildIFO(times ...[4]byte) []byte {
	data := make([]byte, dvdSectorSize)
	copy(data, vtsMagic)
	binary.BigEndian.PutUint32(data[ifoPGCITSectorOffset:], 1)

	pgcit := make([]byte, pgcitHeaderSize+len(times)*pgcSearchPointerSize)
	binary.BigEndian.PutUint16(pgcit, uint16(len(times)))
	for index := range times {
		pointer := pgcitHeaderSize + index*pgcSearchPointerSize
		pgcOffset := len(pgcit) + index*testPGCSize
		binary.BigEndian.PutUint32(pgcit[pointer+4:], uint32(pgcOffset))
	}
	for _, playback := range times {
		pgc := make([]byte, testPGCSize)
		copy(pgc[pgcPlaybackTimeOffset:], playback[:])
		pgcit = append(pgcit, pgc...)
	}
	binary.BigEndian.PutUint32(pgcit[4:], uint32(len(pgcit)-1))

	return append(data, pgcit...)
}

// sized returns a map file of the given size (contents are irrelevant to title selection).
func sized(size int) *fstest.MapFile {
	return &fstest.MapFile{Data: make([]byte, size)}
}

func file(data []byte) *fstest.MapFile {
	return &fstest.MapFile{Data: data}
}
