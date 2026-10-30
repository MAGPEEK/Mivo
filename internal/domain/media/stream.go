package media

import (
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
)

// StreamKind is the type of an elementary stream.
type StreamKind string

// The stream kinds Mivo distinguishes. Data and attachment streams (fonts, timecodes,
// chapters as bin_data) are "other".
const (
	StreamKindVideo    StreamKind = "video"
	StreamKindAudio    StreamKind = "audio"
	StreamKindSubtitle StreamKind = "subtitle"
	StreamKindOther    StreamKind = "other"
)

// ParseStreamKind accepts the exact name of a stream kind, for example when reading a
// stored value back. Anything else wraps domain.ErrInvalid.
func ParseStreamKind(text string) (StreamKind, error) {
	kind := StreamKind(text)
	if !kind.isKnown() {
		return "", fmt.Errorf("parse stream kind %q: %w", text, domain.ErrInvalid)
	}

	return kind, nil
}

func (kind StreamKind) isKnown() bool {
	switch kind {
	case StreamKindVideo, StreamKindAudio, StreamKindSubtitle, StreamKindOther:
		return true
	default:
		return false
	}
}

// Stream is one elementary stream of a file with the values the adapter normalised from
// ffprobe. Text fields are empty and numbers 0 when ffprobe did not report them; fields
// that do not apply to the kind (channels of a video stream) stay zero.
type Stream struct {
	// Index is ffprobe's stream index, unique within the file. FFmpeg selects streams by it.
	Index int
	Kind  StreamKind
	// Codec is ffprobe's codec_name, for example h264, hevc, aac or subrip.
	Codec string
	// CodecTag is the FourCC from the container (XVID, DIVX), shown as a technical hint.
	CodecTag string
	// Profile is ffprobe's profile text, for example "High" or "Main 10".
	Profile string
	// Level is ffprobe's numeric level (H.264 4.0 is 40, HEVC 4.0 is 120); 0 when unknown.
	Level int

	// Video.
	PixelFormat string
	// BitDepth is the bits per colour component, see the BitDepth function.
	BitDepth       int
	Width          int
	Height         int
	FrameRate      FrameRate
	ColorPrimaries string
	ColorTransfer  string
	ColorSpace     string
	// HDR is the result of ClassifyHDR, stored so lists need not recompute it.
	HDR HDRFormat

	// Audio.
	Channels      int
	ChannelLayout string
	SampleRate    int

	// Tags and dispositions.
	Language string
	Title    string
	Default  bool
	Forced   bool
	// AttachedPicture marks cover art stored as a video stream (attached_pic disposition).
	AttachedPicture bool

	// BitRate is in bits per second; 0 when unknown, which is common for Matroska streams.
	BitRate int64
}

// streamNumber names a numeric field for validation messages.
type streamNumber struct {
	name  string
	value int64
}

// validate checks the values of a single stream; uniqueness of indexes is checked by the
// description.
func (stream Stream) validate() error {
	if !stream.Kind.isKnown() {
		return fmt.Errorf("stream %d: unknown kind %q: %w", stream.Index, stream.Kind, domain.ErrInvalid)
	}

	if !stream.HDR.isKnown() {
		return fmt.Errorf("stream %d: unknown hdr format %q: %w", stream.Index, stream.HDR, domain.ErrInvalid)
	}

	for _, number := range stream.numbers() {
		if number.value < 0 {
			return fmt.Errorf("stream %d: negative %s %d: %w",
				stream.Index, number.name, number.value, domain.ErrInvalid)
		}
	}

	return nil
}

func (stream Stream) numbers() []streamNumber {
	return []streamNumber{
		{name: "index", value: int64(stream.Index)},
		{name: "level", value: int64(stream.Level)},
		{name: "bit depth", value: int64(stream.BitDepth)},
		{name: "width", value: int64(stream.Width)},
		{name: "height", value: int64(stream.Height)},
		{name: "channels", value: int64(stream.Channels)},
		{name: "sample rate", value: int64(stream.SampleRate)},
		{name: "bit rate", value: stream.BitRate},
	}
}
