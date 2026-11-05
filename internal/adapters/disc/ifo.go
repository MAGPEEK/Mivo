package disc

import (
	"fmt"
	"time"
)

// VTS IFO layout (DVD-Video title set information, as read by libdvdread's ifo_read).
const (
	vtsMagic = "DVDVIDEO-VTS"
	// ifoPGCITSectorOffset holds the sector of the title set program chain table (VTS_PGCIT).
	ifoPGCITSectorOffset = 0xCC
	dvdSectorSize        = 2048
	// pgcitHeaderSize covers the PGC count, 2 reserved bytes and the end address.
	pgcitHeaderSize = 8
	// pgcSearchPointerSize is one PGCIT entry: 4 bytes category, 4 bytes PGC offset.
	pgcSearchPointerSize     = 8
	pgcSearchPointerOffsetAt = 4
	// pgcPlaybackTimeOffset is the BCD hh:mm:ss:ff field inside a PGC.
	pgcPlaybackTimeOffset = 4
	pgcPlaybackTimeLength = 4

	// maxIFOSize skips implausibly large IFO files; real ones stay below a few hundred KiB.
	maxIFOSize = 4 << 20

	// frameRateShift and the rate codes select the frame rate in the top bits of the frame byte.
	frameRateShift      = 6
	frameRate25Code     = 1
	frameRate30Code     = 3
	framesMask          = 0x3F
	framesPerSecond25   = 25
	framesPerSecond30   = 30
	maxMinutesOrSeconds = 59
)

// parseTitleSetDuration returns the longest program chain playback time of a VTS IFO. A
// title set's VOBs hold all its PGCs; the longest one is the feature (a "play all" PGC equals
// the sum of the episodes it chains). Invalid PGC entries are skipped; offsets and the PGC
// count are bounded by the data, so the loop ends after at most 65535 entries.
func parseTitleSetDuration(data []byte) (time.Duration, error) {
	view := byteView(data)
	magic, err := view.span(0, int64(len(vtsMagic)))
	if err != nil || string(magic) != vtsMagic {
		return 0, fmt.Errorf("%w: not a VTS IFO", errMalformed)
	}
	sector, err := view.u32(ifoPGCITSectorOffset)
	if err != nil {
		return 0, malformed("PGCIT sector", err)
	}
	pgcit := int64(sector) * dvdSectorSize
	count, err := view.u16(pgcit)
	if err != nil {
		return 0, malformed("PGC count", err)
	}

	var longest time.Duration
	found := false
	for index := range int64(count) {
		pointer := pgcit + pgcitHeaderSize + index*pgcSearchPointerSize
		pgcOffset, err := view.u32(pointer + pgcSearchPointerOffsetAt)
		if err != nil {
			break // the table is truncated; later entries cannot be read either
		}
		playback, err := pgcPlaybackTime(view, pgcit+int64(pgcOffset))
		if err != nil {
			continue
		}
		longest = max(longest, playback)
		found = true
	}
	if !found {
		return 0, fmt.Errorf("%w: no readable program chain", errMalformed)
	}

	return longest, nil
}

func pgcPlaybackTime(view byteView, pgc int64) (time.Duration, error) {
	field, err := view.span(pgc+pgcPlaybackTimeOffset, pgcPlaybackTimeLength)
	if err != nil {
		return 0, malformed("PGC playback time", err)
	}

	return decodePlaybackTime([pgcPlaybackTimeLength]byte(field))
}

// decodePlaybackTime decodes BCD hours, minutes, seconds and frames; the top two bits of the
// frame byte give the frame rate (25 or 30 fps). Zero frames need no valid rate.
func decodePlaybackTime(field [pgcPlaybackTimeLength]byte) (time.Duration, error) {
	hours, okHours := decodeBCD(field[0])
	minutes, okMinutes := decodeBCD(field[1])
	seconds, okSeconds := decodeBCD(field[2])
	frames, okFrames := decodeBCD(field[3] & framesMask)
	if !okHours || !okMinutes || !okSeconds || !okFrames ||
		minutes > maxMinutesOrSeconds || seconds > maxMinutesOrSeconds {
		return 0, fmt.Errorf("%w: playback time % x", errMalformed, field)
	}

	whole := time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second
	if frames == 0 {
		return whole, nil
	}
	var rate int
	switch field[3] >> frameRateShift {
	case frameRate25Code:
		rate = framesPerSecond25
	case frameRate30Code:
		rate = framesPerSecond30
	default:
		return 0, fmt.Errorf("%w: frame rate code in % x", errMalformed, field)
	}
	if frames >= rate {
		return 0, fmt.Errorf("%w: %d frames at %d fps", errMalformed, frames, rate)
	}

	return whole + time.Duration(frames)*time.Second/time.Duration(rate), nil
}

// decodeBCD decodes one packed BCD byte (two decimal digits).
func decodeBCD(value byte) (int, bool) {
	const nibble, lowNibbleMask, maxDigit = 4, 0x0F, 9
	high, low := int(value>>nibble), int(value&lowNibbleMask)
	if high > maxDigit || low > maxDigit {
		return 0, false
	}

	return high*decimalBase + low, true
}
