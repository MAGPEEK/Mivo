package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

// DVD-Video structures, as libdvdread and internal/adapters/disc read them. The generator
// writes only the fields a player needs to find the title sets and their playback time; the
// IFO writers are test-corpus code, not a DVD authoring tool.
const (
	dvdSectorSize = 2048
	vmgMagic      = "DVDVIDEO-VMG"
	vtsMagic      = "DVDVIDEO-VTS"
	// ifoVersionOffset holds the DVD specification version (1.1 as 0x0011).
	ifoVersionOffset = 0x20
	ifoVersion       = 0x0011
	// ifoPGCITSectorOffset holds the sector of the title set program chain table (VTS_PGCIT),
	// which the generator places in the IFO's second sector.
	ifoPGCITSectorOffset = 0xCC
	pgcitSector          = 1
	// pgcitHeaderSize covers the PGC count, 2 reserved bytes and the end address.
	pgcitHeaderSize        = 8
	pgcitEndAddressOffset  = 4
	pgcSearchPointerSize   = 8
	pgcSearchPointerOffset = 4
	// pgcEntryTitleOne is the category of an entry PGC of title 1.
	pgcEntryTitleOne = 0x81
	// pgcHeaderSize is the fixed part of a program chain; the playback time is at offset 4.
	pgcHeaderSize         = 0xEC
	pgcPlaybackTimeOffset = 4
	// pal25FPSFlag is the frame-rate code (25 fps) in the top bits of the frame byte.
	pal25FPSFlag = 0x40

	secondsPerMinute = 60
	minutesPerHour   = 60
	bcdTens          = 10
	bcdNibbleShift   = 4
)

// MPEG-2 program stream pack fields used to flag CSS sectors (ISO/IEC 13818-1).
const (
	packStartCode      = 0xBA
	systemHeaderStream = 0xBB
	paddingStream      = 0xBE
	privateStream2     = 0xBF
	packHeaderSize     = 14
	packStuffingOffset = 13
	packStuffingMask   = 0x07
	mpeg2PackMarker    = 0x40
	mpeg2MarkerMask    = 0xC0
	pesFlagsOffset     = 6
	pesMarker          = 0x80
	pesMarkerMask      = 0xC0
	// cssScramblingFlag sets PES_scrambling_control to '01', what a CSS disc carries.
	cssScramblingFlag = 0x10
	startCodePrefix   = 3
)

var errSourceTooSmall = errors.New("the source is too small to split into two parts")

// splitVOB cuts a program stream into two parts at the sector boundary nearest its middle,
// as DVD authoring cuts a title into VTS_nn_1.VOB, VTS_nn_2.VOB, … at 1 GiB.
func splitVOB(stream []byte) ([]byte, []byte, error) {
	middle := len(stream) / dvdSectorSize / halve * dvdSectorSize
	if middle == 0 || len(stream)%dvdSectorSize != 0 {
		return nil, nil, fmt.Errorf("split VOB of %d bytes: %w", len(stream), errSourceTooSmall)
	}

	return stream[:middle], stream[middle:], nil
}

// vmgIFO is a minimal VIDEO_TS.IFO: the signature the scan uses to detect a VIDEO_TS folder.
func vmgIFO() []byte {
	data := make([]byte, dvdSectorSize)
	copy(data, vmgMagic)
	binary.BigEndian.PutUint16(data[ifoVersionOffset:], ifoVersion)

	return data
}

// vtsIFO is a VTS_nn_0.IFO with one program chain of the given playback time (PAL, whole
// seconds) in a PGCIT in its second sector.
func vtsIFO(playback time.Duration) []byte {
	data := make([]byte, dvdSectorSize)
	copy(data, vtsMagic)
	binary.BigEndian.PutUint16(data[ifoVersionOffset:], ifoVersion)
	binary.BigEndian.PutUint32(data[ifoPGCITSectorOffset:], pgcitSector)

	pgcit := make([]byte, pgcitHeaderSize+pgcSearchPointerSize+pgcHeaderSize)
	binary.BigEndian.PutUint16(pgcit, 1)
	binary.BigEndian.PutUint32(pgcit[pgcitEndAddressOffset:], uint32(len(pgcit)-1)) //nolint:gosec // fixed size

	pointer := pgcit[pgcitHeaderSize:]
	pointer[0] = pgcEntryTitleOne
	binary.BigEndian.PutUint32(pointer[pgcSearchPointerOffset:], pgcitHeaderSize+pgcSearchPointerSize)

	pgc := pgcit[pgcitHeaderSize+pgcSearchPointerSize:]
	copy(pgc[pgcPlaybackTimeOffset:], playbackTimeBCD(playback))

	return append(data, padToSector(pgcit)...)
}

// playbackTimeBCD encodes whole hours, minutes and seconds as BCD, zero frames at 25 fps.
func playbackTimeBCD(playback time.Duration) []byte {
	seconds := int(playback / time.Second)
	minutes := seconds / secondsPerMinute

	return []byte{
		bcd(minutes / minutesPerHour), bcd(minutes % minutesPerHour), bcd(seconds % secondsPerMinute), pal25FPSFlag,
	}
}

func bcd(value int) byte {
	return byte(value/bcdTens<<bcdNibbleShift | value%bcdTens) //nolint:gosec // two decimal digits
}

func padToSector(data []byte) []byte {
	if rest := len(data) % dvdSectorSize; rest != 0 {
		data = append(data, make([]byte, dvdSectorSize-rest)...)
	}

	return data
}

// flagCSSSectors returns a copy of a VOB in which every pack that starts with an elementary
// stream PES packet has PES_scrambling_control set, as on a CSS-protected disc. Only the flag
// changes: the payload stays the clear stream, there is no key and no cipher. It fails when
// no pack was flagged, because the stand-in would then look clear.
func flagCSSSectors(vob []byte) ([]byte, error) {
	flagged := append([]byte(nil), vob...)
	count := 0

	for start := 0; start+dvdSectorSize <= len(flagged); start += dvdSectorSize {
		if flagPack(flagged[start : start+dvdSectorSize]) {
			count++
		}
	}

	if count == 0 {
		return nil, errors.New("flag CSS sectors: the VOB holds no elementary stream pack")
	}

	return flagged, nil
}

// flagPack sets the scrambling flag of the first PES packet of an MPEG-2 pack.
func flagPack(sector []byte) bool {
	if !hasStartCode(sector, 0) || sector[startCodePrefix] != packStartCode ||
		sector[startCodePrefix+1]&mpeg2MarkerMask != mpeg2PackMarker {
		return false
	}

	pes := packHeaderSize + int(sector[packStuffingOffset]&packStuffingMask)
	if !hasStartCode(sector, pes) {
		return false
	}

	switch sector[pes+startCodePrefix] {
	case systemHeaderStream, paddingStream, privateStream2:
		return false
	}

	flags := &sector[pes+pesFlagsOffset]
	if *flags&pesMarkerMask != pesMarker {
		return false
	}

	*flags |= cssScramblingFlag

	return true
}

// hasStartCode reports the MPEG start code prefix 00 00 01 and a stream id byte at offset.
func hasStartCode(data []byte, offset int) bool {
	return offset+startCodePrefix < len(data) && data[offset] == 0 && data[offset+1] == 0 && data[offset+2] == 1
}
