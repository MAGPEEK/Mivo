package ffmpeg

import (
	"bytes"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/magpeek/mivo/internal/domain"
)

const (
	// webVTTFormat is both the muxer and the encoder that write the converted cues. The
	// encoder drops ASS styling and override tags, which WebVTT cannot express (ADR 0030 §3).
	webVTTFormat = "webvtt"
	// charsetOption tells FFmpeg's subtitle decoder the encoding of a text file; its
	// demuxers convert a UTF-16 or UTF-32 file with a BOM on their own and need none.
	charsetOption = "-sub_charenc"
	// windows1252 is the fallback for text that is neither UTF-8 nor marked by a BOM: the
	// legacy encoding of Western European .srt files.
	windows1252 = "CP1252"
	// subtitleStreamSelector maps the n-th subtitle stream of the first input.
	subtitleStreamSelector = "0:s:"
	// firstSubtitleStream selects the only subtitle stream of an external file.
	firstSubtitleStream = "0:s:0"
	// webVTTSignature starts every WebVTT file.
	webVTTSignature = "WEBVTT"
)

// byteOrderMarks are the marks FFmpeg's text subtitle demuxers understand: UTF-8, UTF-32 and
// UTF-16 in both byte orders.
func byteOrderMarks() [][]byte {
	return [][]byte{
		{0xEF, 0xBB, 0xBF},
		{0xFF, 0xFE, 0x00, 0x00},
		{0x00, 0x00, 0xFE, 0xFF},
		{0xFF, 0xFE},
		{0xFE, 0xFF},
	}
}

// imageSubtitleCodecs are the bitmap subtitle codecs (ffprobe names): their cues are
// pictures that cannot become text without OCR, so a player burns them in instead.
func imageSubtitleCodecs() []string {
	return []string{"hdmv_pgs_subtitle", "dvd_subtitle", "dvb_subtitle", "xsub", "dvb_teletext"}
}

// imageSubtitleExtensions are the file extensions of bitmap subtitle files.
func imageSubtitleExtensions() []string {
	return []string{".sup", ".sub", ".idx"}
}

// textSubtitleDemuxers maps the extension of an external text subtitle file to the demuxer
// that reads it. Every entry is on the demuxer whitelist (allowedDemuxers).
func textSubtitleDemuxers() map[string]string {
	return map[string]string{".srt": "srt", ".ass": "ass", ".ssa": "ass", ".vtt": "webvtt"}
}

// embeddedSubtitleArguments builds the ffmpeg arguments that convert the ordinal-th subtitle
// stream (counted among subtitle streams only) of the media file at path to WebVTT on
// standard output. Errors wrap domain.ErrInvalid for a negative ordinal, a relative or a
// Windows device path, and for a bitmap codec.
func embeddedSubtitleArguments(path string, ordinal int, codec string) ([]string, error) {
	if ordinal < 0 {
		return nil, fmt.Errorf("subtitle stream %d is negative: %w", ordinal, domain.ErrInvalid)
	}

	if slices.Contains(imageSubtitleCodecs(), codec) {
		return nil, fmt.Errorf("%s subtitles are images, not text: %w", codec, domain.ErrInvalid)
	}

	input, err := ffmpegInputArguments(path)
	if err != nil {
		return nil, err
	}

	arguments := []string{"-hide_banner", "-nostdin", "-loglevel", "error"}
	arguments = append(arguments, input...)

	return slices.Concat(arguments, subtitleOutputArguments(subtitleStreamSelector+strconv.Itoa(ordinal))), nil
}

// externalSubtitleArguments builds the ffmpeg arguments that convert a text subtitle file of
// demuxer format, piped through standard input, to WebVTT on standard output. The file is
// piped because the caller already read it, bounded and confined to its root, to sniff the
// encoding; FFmpeg never opens the path itself. charset is empty for UTF-8.
func externalSubtitleArguments(format, charset string) []string {
	arguments := []string{
		"-hide_banner", "-loglevel", "error",
		formatWhitelistOption, format,
		protocolWhitelistOption, pipeProtocol,
	}

	if charset != "" {
		arguments = append(arguments, charsetOption, charset)
	}

	arguments = append(arguments, "-f", format, "-i", standardInput)

	return slices.Concat(arguments, subtitleOutputArguments(firstSubtitleStream))
}

// subtitleOutputArguments selects stream and encodes it as WebVTT on standard output.
func subtitleOutputArguments(stream string) []string {
	return []string{
		"-map", stream, "-map_metadata", disabledMapping, "-c:s", webVTTFormat, "-f", webVTTFormat, standardOutput,
	}
}

// externalSubtitleFormat returns the demuxer of an external subtitle file. Errors wrap
// domain.ErrInvalid for a bitmap subtitle file and domain.ErrUnsupported for an unknown one.
func externalSubtitleFormat(path string) (string, error) {
	extension := strings.ToLower(filepath.Ext(path))

	if slices.Contains(imageSubtitleExtensions(), extension) {
		return "", fmt.Errorf("%s subtitle files are images, not text: %w", extension, domain.ErrInvalid)
	}

	format, known := textSubtitleDemuxers()[extension]
	if !known {
		return "", fmt.Errorf("subtitle file extension %q: %w", extension, domain.ErrUnsupported)
	}

	return format, nil
}

// subtitleCharset returns the -sub_charenc value for a text subtitle file, or "" when none
// is needed. Only a byte order mark and UTF-8 validity decide; nothing guesses further. A
// file with a BOM is converted by FFmpeg's demuxer itself, so naming its encoding again
// would convert twice. Text that is no valid UTF-8 is read as Windows-1252, which maps every
// byte to some character and so never fails.
func subtitleCharset(data []byte) string {
	for _, mark := range byteOrderMarks() {
		if bytes.HasPrefix(data, mark) {
			return ""
		}
	}

	if utf8.Valid(data) {
		return ""
	}

	return windows1252
}
