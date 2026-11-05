package main

import (
	"encoding/binary"
	"time"
)

// FFmpeg cannot render text into bitmap subtitles (its subtitle encoders convert text to text
// and bitmap to bitmap only), and lavfi has no subtitle source. The disc corpus therefore
// writes a raw HDMV PGS stream (FFmpeg's "sup" demuxer) itself: every cue shows one filled
// rectangle. Blu-ray clips copy it as hdmv_pgs_subtitle; DVD titles re-encode it with the
// dvdsub encoder (bitmap to bitmap) as dvd_subtitle.

// PGS segment layout (Blu-ray "Presentation Graphic Stream", as FFmpeg's pgssubdec reads it).
const (
	pgsMagic = "PG"
	// pgsClockRate is the 90 kHz clock of the segment PTS.
	pgsClockRate = 90000

	pgsSegmentPalette     = 0x14
	pgsSegmentObject      = 0x15
	pgsSegmentComposition = 0x16
	pgsSegmentWindow      = 0x17
	pgsSegmentEnd         = 0x80

	// pgsFrameRateCode is the frame rate field of a composition; decoders ignore it.
	pgsFrameRateCode = 0x10
	// pgsEpochStart starts a new display set with its own objects; pgsNormalCase clears it.
	pgsEpochStart = 0x80
	pgsNormalCase = 0x00
	// pgsObjectComplete marks an object that fits in one segment (first and last fragment).
	pgsObjectComplete = 0xC0
	// pgsObjectSizeFields is the width and height that the object data length includes.
	pgsObjectSizeFields = 4

	// The rectangle: palette entry 1 in white (Y, Cr, Cb, alpha), centred near the bottom.
	pgsColorIndex     = 1
	pgsWhiteLuma      = 235
	pgsNeutralChroma  = 128
	pgsOpaque         = 255
	pgsRectangleWidth = 192
	// pgsRectangleHeight is the rectangle height in lines.
	pgsRectangleHeight = 32
	// pgsBottomMargin keeps the rectangle off the bottom edge, like a subtitle line.
	pgsBottomMargin = 48

	// The run-length codes: 00 Cl ll cc is a run of l (64..16383) pixels of colour c, 00 00
	// ends a line.
	pgsRunEscape     = 0x00
	pgsLongColorRun  = 0xC0
	pgsRunHighShift  = 8
	pgsEndOfLineCode = 0x00
	byteMask         = 0xFF
	// uint24FieldStart skips the high byte of a uint32 that holds a 24-bit field.
	uint24FieldStart = 1
	// halve centres a length.
	halve = 2
)

// bitmapCue is one subtitle event shown from start to end.
type bitmapCue struct {
	start, end time.Duration
}

// pgsFrame is the video frame size the compositions refer to.
type pgsFrame struct {
	width, height uint16
}

// pgsSubtitles returns a raw PGS stream for the frame: one display set that shows the
// rectangle at each cue start and one that clears it at the cue end.
func pgsSubtitles(frame pgsFrame, cues []bitmapCue) []byte {
	var stream []byte

	composition := uint16(0)

	for _, cue := range cues {
		stream = append(stream, frame.displaySet(pgsTimestamp(cue.start), composition, true)...)
		composition++
		stream = append(stream, frame.displaySet(pgsTimestamp(cue.end), composition, false)...)
		composition++
	}

	return stream
}

func pgsTimestamp(position time.Duration) uint32 {
	return uint32(position * pgsClockRate / time.Second) //nolint:gosec // cue positions are seconds
}

// origin centres the rectangle horizontally and keeps it above the bottom margin.
func (frame pgsFrame) origin() (uint16, uint16) {
	return (frame.width - pgsRectangleWidth) / halve, frame.height - pgsRectangleHeight - pgsBottomMargin
}

// displaySet writes composition, window, (palette and object when shown) and end segments.
func (frame pgsFrame) displaySet(pts uint32, composition uint16, show bool) []byte {
	x, y := frame.origin()

	set := pgsSegment(pts, pgsSegmentComposition, frame.composition(composition, show))
	set = append(set, pgsSegment(pts, pgsSegmentWindow, pgsWindow(x, y))...)

	if show {
		palette := []byte{0, 0, pgsColorIndex, pgsWhiteLuma, pgsNeutralChroma, pgsNeutralChroma, pgsOpaque}
		set = append(set, pgsSegment(pts, pgsSegmentPalette, palette)...)
		set = append(set, pgsSegment(pts, pgsSegmentObject, pgsObject())...)
	}

	return append(set, pgsSegment(pts, pgsSegmentEnd, nil)...)
}

// pgsSegment frames a segment: magic, PTS, DTS (0), type and length.
func pgsSegment(pts uint32, segmentType byte, payload []byte) []byte {
	segment := []byte(pgsMagic)
	segment = binary.BigEndian.AppendUint32(segment, pts)
	segment = binary.BigEndian.AppendUint32(segment, 0)
	segment = append(segment, segmentType)
	segment = binary.BigEndian.AppendUint16(segment, uint16(len(payload))) //nolint:gosec // segments stay below 64 KiB

	return append(segment, payload...)
}

func (frame pgsFrame) composition(composition uint16, show bool) []byte {
	payload := binary.BigEndian.AppendUint16(nil, frame.width)
	payload = binary.BigEndian.AppendUint16(payload, frame.height)
	payload = append(payload, pgsFrameRateCode)
	payload = binary.BigEndian.AppendUint16(payload, composition)

	if !show {
		// Normal case, no palette update, palette 0, no objects: clears the window.
		return append(payload, pgsNormalCase, 0, 0, 0)
	}

	// Epoch start, no palette update, palette 0, one object: id 0, window 0, not cropped.
	x, y := frame.origin()
	payload = append(payload, pgsEpochStart, 0, 0, 1, 0, 0, 0, 0)
	payload = binary.BigEndian.AppendUint16(payload, x)

	return binary.BigEndian.AppendUint16(payload, y)
}

func pgsWindow(x, y uint16) []byte {
	payload := []byte{1, 0} // one window, id 0
	payload = binary.BigEndian.AppendUint16(payload, x)
	payload = binary.BigEndian.AppendUint16(payload, y)
	payload = binary.BigEndian.AppendUint16(payload, pgsRectangleWidth)

	return binary.BigEndian.AppendUint16(payload, pgsRectangleHeight)
}

// pgsObject is object 0, version 0, in one fragment: the run-length coded rectangle.
func pgsObject() []byte {
	var lines []byte
	for range pgsRectangleHeight {
		lines = append(lines, pgsRunEscape, pgsLongColorRun|byte(pgsRectangleWidth>>pgsRunHighShift),
			byte(pgsRectangleWidth&byteMask), pgsColorIndex, pgsRunEscape, pgsEndOfLineCode)
	}

	// The object data length is a 24-bit field: the low three bytes of a big-endian uint32
	// (the rectangle needs a few hundred bytes).
	length := binary.BigEndian.AppendUint32(nil, uint32(len(lines)+pgsObjectSizeFields)) //nolint:gosec // a few hundred bytes
	payload := append([]byte{0, 0, 0, pgsObjectComplete}, length[uint24FieldStart:]...)
	payload = binary.BigEndian.AppendUint16(payload, pgsRectangleWidth)
	payload = binary.BigEndian.AppendUint16(payload, pgsRectangleHeight)

	return append(payload, lines...)
}
