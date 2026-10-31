package ffmpeg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

const (
	// probeTimeout bounds one ffprobe run. Probing reads only the first megabytes of a file,
	// but a NAS disk may have to spin up first.
	probeTimeout = 30 * time.Second
	// ffprobeName names the tool in errors.
	ffprobeName = "ffprobe"
	// fileProtocol makes FFmpeg read every input as a local file, so a name such as
	// "concat:x.mkv" or "-i.mkv" is never taken as a protocol or an option (ADR 0022).
	fileProtocol = "file:"
	// protocolWhitelistOption with allowedProtocols limits every URL FFmpeg opens for an
	// input, including the nested opens of demuxers chosen by the file's content (a DASH
	// manifest, an HLS playlist or a concat list named .mkv): file: protects the name only,
	// the whitelist also protects against the content reaching the network (ADR 0022).
	// FFmpeg's default for file: inputs ("file,crypto,data") already excludes the network;
	// the explicit list does not depend on that default.
	protocolWhitelistOption = "-protocol_whitelist"
	allowedProtocols        = "file"
	// formatWhitelistOption with allowedDemuxers limits the demuxers FFmpeg may pick for an
	// input, whatever the file's content says. FFmpeg checks it after probing and before the
	// demuxer reads its header, which is where playlist-like demuxers (concat/ffconcat, hls,
	// dash, imf) open the files they reference, and copies it into nested opens (ADR 0022
	// Notes M2-24b). A refused input fails with "Format not on whitelist".
	formatWhitelistOption = "-format_whitelist"
	// demuxerSeparator joins the entries of the whitelist.
	demuxerSeparator = ","
	// imageSequenceDemuxer reads numbered image sequences ("img%03d.jpg") and, in FFmpeg
	// 7.1, %-escaped globs: both need a "%" in the path. Without one it reads exactly the
	// named file, the only way FFmpeg opens a JPEG, so it is allowed for such paths only.
	imageSequenceDemuxer = "image2"
	// imageSequenceMarker starts every image2 pattern.
	imageSequenceMarker = "%"
	// undeterminedLanguage is the ISO 639-2 code muxers write for "no language"; it carries
	// no information, so it is stored as an empty language.
	undeterminedLanguage = "und"
)

// allowedDemuxers are the FFmpeg demuxers that read exactly the one file they are given: the
// containers Mivo knows (media.IdentifyContainer) and the scanner's video extensions map to,
// the audio containers an audio-only file is remuxed from, single images and text or bitmap
// subtitle files. Missing on purpose, because they open further files named by the content
// or derived from the path: concat (ffconcat lists), hls, dash, imf, vobsub (the companion
// .sub file), image2 (see imageSequenceDemuxer); also every device, script (avisynth,
// vapoursynth) and network demuxer. A file whose content selects one of them is refused,
// never handed to the next-best demuxer.
func allowedDemuxers() []string {
	return []string{
		// Video containers.
		"matroska", "mov", "mpegts", "mpeg", "mpegvideo", "avi", "flv", "live_flv", "ogg", "asf", "mxf", "rm",
		// Audio containers and raw audio streams.
		"flac", "mp3", "wav", "aiff", "wv", "ape", "dsf", "iff", "aac", "ac3", "eac3", "dts",
		// Single images.
		"gif", "png_pipe", "jpeg_pipe", "tiff_pipe", "webp_pipe",
		// Subtitle files.
		"srt", "webvtt", "ass", "sup",
	}
}

// inputDemuxers returns the -format_whitelist value for absolutePath. A whitelist entry
// matches every alias of a demuxer ("matroska" matches "matroska,webm").
func inputDemuxers(absolutePath string) string {
	demuxers := allowedDemuxers()
	if !strings.Contains(absolutePath, imageSequenceMarker) {
		demuxers = append(demuxers, imageSequenceDemuxer)
	}

	return strings.Join(demuxers, demuxerSeparator)
}

// placeholderValues are what ffprobe prints instead of a missing text value.
func placeholderValues() []string {
	return []string{"unknown", "N/A"}
}

// Prober describes media files with ffprobe and finds keyframes for stream-copy seeks.
type Prober struct {
	ffprobe         tool
	probeTimeout    time.Duration
	keyframeTimeout time.Duration
}

// NewProber returns a prober that runs the ffprobe executable at ffprobePath, the
// configured tools.ffprobe value resolved by the caller.
func NewProber(ffprobePath string) *Prober {
	return &Prober{
		ffprobe:         tool{name: ffprobeName, executable: ffprobePath},
		probeTimeout:    probeTimeout,
		keyframeTimeout: keyframeTimeout,
	}
}

// Probe describes the file at absolutePath. The caller has resolved the path and checked
// that it lies inside a library root (ADR 0022). Errors wrap
//   - domain.ErrInvalid for a relative path,
//   - context.DeadlineExceeded when ffprobe did not finish within the per-file timeout (or
//     the caller's deadline), context.Canceled when the caller gave up,
//   - domain.ErrUnavailable when ffprobe cannot be started,
//   - domain.ErrUnsupported when ffprobe failed on the file or its output was unusable.
//
// Error messages name the file by its base name only, so logs do not reveal library roots.
func (p *Prober) Probe(ctx context.Context, absolutePath string) (media.Description, error) {
	input, err := inputArguments(absolutePath)
	if err != nil {
		return media.Description{}, fmt.Errorf("probe: %w", err)
	}

	probeContext, cancel := context.WithTimeout(ctx, p.probeTimeout)
	defer cancel()

	output, err := p.ffprobe.output(probeContext, probeArguments(input))
	if err != nil {
		return media.Description{}, fmt.Errorf("probe %s: %w", filepath.Base(absolutePath), classifyRunError(err))
	}

	description, err := describe(output, filepath.Ext(absolutePath))
	if err != nil {
		return media.Description{}, fmt.Errorf("probe %s: %w", filepath.Base(absolutePath), err)
	}

	return description, nil
}

// probeArguments ends with input, the result of inputArguments.
func probeArguments(input []string) []string {
	return append([]string{
		"-v", "error", "-hide_banner", "-print_format", "json", "-show_format", "-show_streams",
	}, input...)
}

// inputArguments returns the arguments that open absolutePath as FFmpeg's input: the format
// and protocol whitelists followed by the file: URL. Every tool invocation that reads a
// media file ends with them. Relative paths are rejected because they would depend on the
// server's working directory, Windows device paths because they do not name a file.
func inputArguments(absolutePath string) ([]string, error) {
	if !filepath.IsAbs(absolutePath) {
		return nil, fmt.Errorf("path %q is not absolute: %w", absolutePath, domain.ErrInvalid)
	}

	if isWindowsDevicePath(absolutePath) {
		return nil, fmt.Errorf("path in the Windows device namespace: %w", domain.ErrInvalid)
	}

	return []string{
		formatWhitelistOption, inputDemuxers(absolutePath),
		protocolWhitelistOption, allowedProtocols,
		fileProtocol + absolutePath,
	}, nil
}

// isWindowsDevicePath reports paths such as \\.\COM1 or \\?\GLOBALROOT\Device\... that
// reach devices instead of files. Long paths (\\?\C:\...) and UNC shares stay allowed.
func isWindowsDevicePath(path string) bool {
	if runtime.GOOS != "windows" {
		return false
	}

	normalised := strings.ToUpper(strings.ReplaceAll(path, "/", `\`))

	return strings.HasPrefix(normalised, `\\.\`) || strings.HasPrefix(normalised, `\\?\GLOBALROOT`)
}

// classifyRunError marks failures caused by the file as unsupported. Context errors and a
// tool that cannot start stay as they are: they say nothing about the file.
func classifyRunError(err error) error {
	var exit *exitError
	if errors.As(err, &exit) || errors.Is(err, errOutputTooLarge) {
		return fmt.Errorf("%w: %w", domain.ErrUnsupported, err)
	}

	return err
}

// probeOutput is the part of ffprobe's -show_format -show_streams JSON Mivo reads. Numbers
// that ffprobe prints as strings (and as "N/A" when unknown) stay strings here.
type probeOutput struct {
	Streams []probeStream `json:"streams"`
	Format  *probeFormat  `json:"format"`
}

type probeFormat struct {
	FormatName string `json:"format_name"`
	Duration   string `json:"duration"`
	BitRate    string `json:"bit_rate"`
}

type probeStream struct {
	Index            int               `json:"index"`
	CodecName        string            `json:"codec_name"`
	CodecType        string            `json:"codec_type"`
	CodecTagString   string            `json:"codec_tag_string"`
	Profile          string            `json:"profile"`
	Level            int               `json:"level"`
	PixelFormat      string            `json:"pix_fmt"`
	BitsPerRawSample string            `json:"bits_per_raw_sample"`
	Width            int               `json:"width"`
	Height           int               `json:"height"`
	AverageFrameRate string            `json:"avg_frame_rate"`
	RealFrameRate    string            `json:"r_frame_rate"`
	ColorPrimaries   string            `json:"color_primaries"`
	ColorTransfer    string            `json:"color_transfer"`
	ColorSpace       string            `json:"color_space"`
	Channels         int               `json:"channels"`
	ChannelLayout    string            `json:"channel_layout"`
	SampleRate       string            `json:"sample_rate"`
	BitRate          string            `json:"bit_rate"`
	Disposition      map[string]int    `json:"disposition"`
	Tags             map[string]string `json:"tags"`
}

// describe maps ffprobe's JSON to the domain. extension is the file extension (with or
// without the dot) that refines the container inside a format family. Errors wrap
// domain.ErrUnsupported.
func describe(output []byte, extension string) (media.Description, error) {
	var parsed probeOutput
	if err := json.Unmarshal(output, &parsed); err != nil {
		return media.Description{}, fmt.Errorf("decode ffprobe output: %w: %w", domain.ErrUnsupported, err)
	}

	if parsed.Format == nil {
		return media.Description{}, fmt.Errorf("ffprobe output has no format section: %w", domain.ErrUnsupported)
	}

	duration, _ := parseSeconds(parsed.Format.Duration)
	description := media.Description{
		Container:  media.IdentifyContainer(parsed.Format.FormatName, extension),
		FormatName: parsed.Format.FormatName,
		Duration:   max(duration, 0),
		BitRate:    parseCount(parsed.Format.BitRate),
		Streams:    make([]media.Stream, 0, len(parsed.Streams)),
	}

	for _, stream := range parsed.Streams {
		description.Streams = append(description.Streams, stream.toDomain())
	}

	// ffprobe never reports what Validate rejects; failing here means its output changed.
	if err := description.Validate(); err != nil {
		return media.Description{}, fmt.Errorf("ffprobe output: %w: %w", domain.ErrUnsupported, err)
	}

	return description, nil
}

func (s probeStream) toDomain() media.Stream {
	stream := media.Stream{
		Index:    s.Index,
		Kind:     streamKind(s.CodecType),
		Codec:    s.CodecName,
		CodecTag: codecTag(s.CodecTagString),
		Profile:  knownText(s.Profile),
		// ffprobe reports an unknown level as -99.
		Level:           max(s.Level, 0),
		Language:        language(s.Tags),
		Title:           tag(s.Tags, "title"),
		Default:         s.Disposition["default"] != 0,
		Forced:          s.Disposition["forced"] != 0,
		AttachedPicture: s.Disposition["attached_pic"] != 0,
		BitRate:         parseCount(s.BitRate),
	}

	switch stream.Kind {
	case media.StreamKindVideo:
		s.fillVideo(&stream)
	case media.StreamKindAudio:
		s.fillAudio(&stream)
	case media.StreamKindSubtitle, media.StreamKindOther:
		// Only codec, tags and dispositions apply.
	}

	return stream
}

func (s probeStream) fillVideo(stream *media.Stream) {
	stream.PixelFormat = knownText(s.PixelFormat)
	stream.BitDepth = media.BitDepth(parseSmallCount(s.BitsPerRawSample), stream.PixelFormat)
	stream.Width = max(s.Width, 0)
	stream.Height = max(s.Height, 0)
	stream.FrameRate = frameRate(s.AverageFrameRate, s.RealFrameRate)
	stream.ColorPrimaries = knownText(s.ColorPrimaries)
	stream.ColorTransfer = knownText(s.ColorTransfer)
	stream.ColorSpace = knownText(s.ColorSpace)
	stream.HDR = media.ClassifyHDR(*stream)
}

// fillAudio also sets BitDepth: bits_per_raw_sample is the sample depth of lossless audio
// (16 or 24 bit FLAC); lossy codecs do not report one.
func (s probeStream) fillAudio(stream *media.Stream) {
	stream.Channels = max(s.Channels, 0)
	stream.ChannelLayout = knownText(s.ChannelLayout)
	stream.SampleRate = parseSmallCount(s.SampleRate)
	stream.BitDepth = media.BitDepth(parseSmallCount(s.BitsPerRawSample), "")
}

func streamKind(codecType string) media.StreamKind {
	switch codecType {
	case "video":
		return media.StreamKindVideo
	case "audio":
		return media.StreamKindAudio
	case "subtitle":
		return media.StreamKindSubtitle
	default:
		return media.StreamKindOther
	}
}

// frameRate prefers the average frame rate (avg_frame_rate), which is what players show,
// and falls back to the base rate (r_frame_rate). A value that does not parse (for example
// "25/0" from a broken header) makes the rate unknown instead of failing the whole probe.
func frameRate(average, base string) media.FrameRate {
	for _, text := range []string{average, base} {
		if rate, err := media.ParseFrameRate(text); err == nil && rate.IsKnown() {
			return rate
		}
	}

	return media.FrameRate{}
}

// codecTag returns the FourCC text of codec_tag_string (XVID, avc1). ffprobe writes
// non-printable bytes in brackets ("[0][0][0][0]", "[27][0][0][0]"); such a tag is a numeric
// container ID, not a FourCC, and is dropped.
func codecTag(text string) string {
	if strings.Contains(text, "[") {
		return ""
	}

	return strings.TrimSpace(text)
}

func language(tags map[string]string) string {
	value := tag(tags, "language")
	if value == undeterminedLanguage {
		return ""
	}

	return value
}

// tag looks a tag up case-insensitively: Matroska files written by other tools use upper-case
// keys (LANGUAGE, TITLE). Among several spellings the exact key, then the smallest key wins,
// so the result does not depend on map order.
func tag(tags map[string]string, key string) string {
	if value, found := tags[key]; found {
		return knownText(value)
	}

	var matches []string

	for candidate := range tags {
		if strings.EqualFold(candidate, key) {
			matches = append(matches, candidate)
		}
	}

	if len(matches) == 0 {
		return ""
	}

	return knownText(tags[slices.Min(matches)])
}

// knownText trims a text value and drops ffprobe's placeholders for unknown values.
func knownText(text string) string {
	trimmed := strings.TrimSpace(text)
	if slices.Contains(placeholderValues(), trimmed) {
		return ""
	}

	return trimmed
}

// parseCount reads a non-negative integer that ffprobe prints as a string. "N/A", malformed
// and negative values yield 0 (unknown).
func parseCount(text string) int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	if err != nil || value < 0 {
		return 0
	}

	return value
}

// parseSmallCount is parseCount for values that fit an int (sample rates, bit depths).
func parseSmallCount(text string) int {
	value, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || value < 0 {
		return 0
	}

	return value
}

// parseSeconds reads ffprobe's decimal seconds ("2.005000", "-0.080000") exactly, without a
// float round trip. The second result is false for "N/A" and anything that is not a plain
// decimal number.
func parseSeconds(text string) (time.Duration, bool) {
	trimmed := strings.TrimSpace(text)
	if !isDecimal(strings.TrimPrefix(trimmed, "-")) {
		return 0, false
	}

	duration, err := time.ParseDuration(trimmed + "s")
	if err != nil {
		return 0, false
	}

	return duration, true
}

// isDecimal accepts digits with an optional fractional part ("12", "12.5").
func isDecimal(text string) bool {
	whole, fraction, hasFraction := strings.Cut(text, ".")

	return isDigits(whole) && (!hasFraction || isDigits(fraction))
}

func isDigits(text string) bool {
	return text != "" && strings.Trim(text, "0123456789") == ""
}
