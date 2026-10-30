package main

import (
	"fmt"
	"slices"
	"strconv"
	"time"
)

// Categories group cases in the output directory and in case names.
const (
	categoryVideo = "video"
)

// Source defaults shared by every clip (ADR 0021: short, rights-free lavfi sources).
const (
	clipDuration      = 2 * time.Second
	defaultWidth      = 320
	defaultHeight     = 240
	defaultFrameRate  = 25
	defaultSampleRate = 48000
	stereoChannels    = 2
	sineFrequencyHz   = 440
)

// mediaCase declares one corpus file as data.
//
// Several cases may share a Name. They are alternatives in order of preference (for example
// different AV1 encoders) and must share Output and Category; the first one the local FFmpeg
// can build is used.
type mediaCase struct {
	// Name identifies the case in the manifest and in the -only filter. By convention it is
	// "<category>-<container>-<codecs>", for example "video-mkv-h264-aac".
	Name     string
	Category string
	// Output is the file path relative to the output directory, always with forward slashes.
	Output string
	Source sourceSpec
	// Args are the encoder options of the case. They are placed after the generic
	// reproducibility options, so a case can override those.
	Args []string
	// Encoders are FFmpeg encoder names (as listed by `ffmpeg -encoders`) the case needs.
	Encoders []string
	// Muxer is the FFmpeg muxer name passed to -f; it is required as well.
	Muxer  string
	Expect expectation
}

// sourceSpec describes the lavfi inputs of a case. A zero Width means no video input and a
// zero SampleRate means no audio input.
type sourceSpec struct {
	Width      int
	Height     int
	FrameRate  int
	SampleRate int
	Channels   int
	Duration   time.Duration
}

// expectation lists the properties ffprobe should report for the generated file. Dimensions,
// sample rate and channel count are derived from the source when the manifest is written.
type expectation struct {
	// Container is the short container name Mivo uses, e.g. "mkv", "m2ts" or "webm".
	Container string `json:"container"`
	// ProbeFormat is the format_name ffprobe reports, e.g. "matroska,webm".
	ProbeFormat string            `json:"probeFormat"`
	Video       *videoExpectation `json:"video,omitempty"`
	Audio       *audioExpectation `json:"audio,omitempty"`
}

// videoExpectation describes the expected video stream. Codec names use ffprobe's codec_name.
type videoExpectation struct {
	Codec          string `json:"codec"`
	Profile        string `json:"profile,omitempty"`
	CodecTag       string `json:"codecTag,omitempty"`
	PixelFormat    string `json:"pixelFormat,omitempty"`
	ColorPrimaries string `json:"colorPrimaries,omitempty"`
	ColorTransfer  string `json:"colorTransfer,omitempty"`
	// HDR names the HDR format whose metadata the stream carries, e.g. "hdr10".
	HDR    string `json:"hdr,omitempty"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// audioExpectation describes the expected audio stream.
type audioExpectation struct {
	Codec      string `json:"codec"`
	SampleRate int    `json:"sampleRate"`
	Channels   int    `json:"channels"`
}

// allCases is the registry of every case category. A new category adds one line here.
func allCases() []mediaCase {
	return slices.Concat(
		videoCases(),
	)
}

// standardSource is the default 2 s, 320x240, 25 fps clip with a 48 kHz stereo sine tone.
func standardSource() sourceSpec {
	return sourceSpec{
		Width:      defaultWidth,
		Height:     defaultHeight,
		FrameRate:  defaultFrameRate,
		SampleRate: defaultSampleRate,
		Channels:   stereoChannels,
		Duration:   clipDuration,
	}
}

// withSize returns the source with another frame size, for codecs with fixed sizes.
func (s sourceSpec) withSize(width, height int) sourceSpec {
	s.Width = width
	s.Height = height

	return s
}

// withAudio returns the source with another audio format, for codecs with fixed rates.
func (s sourceSpec) withAudio(sampleRate, channels int) sourceSpec {
	s.SampleRate = sampleRate
	s.Channels = channels

	return s
}

func (s sourceSpec) hasVideo() bool { return s.Width > 0 && s.Height > 0 }

func (s sourceSpec) hasAudio() bool { return s.SampleRate > 0 }

// expected returns the case expectation completed with the properties of its source.
func (c mediaCase) expected() expectation {
	result := c.Expect

	if result.Video != nil {
		video := *result.Video
		video.Width = c.Source.Width
		video.Height = c.Source.Height
		result.Video = &video
	}

	if result.Audio != nil {
		audio := *result.Audio
		audio.SampleRate = c.Source.SampleRate
		audio.Channels = c.Source.Channels
		result.Audio = &audio
	}

	return result
}

// buildArgs returns the complete FFmpeg argument list that writes the case to outputPath.
func buildArgs(c mediaCase, outputPath string) []string {
	args := []string{
		"-hide_banner", "-nostdin", "-loglevel", "error",
		// The generator always writes to a temporary file it owns and renames it afterwards,
		// so overwriting never touches a finished corpus file.
		"-y",
	}
	args = append(args, sourceArgs(c.Source)...)
	// Strip metadata and encoder version strings so repeated runs give identical files.
	args = append(args, "-map_metadata", "-1", "-fflags", "+bitexact", "-flags", "+bitexact")
	args = append(args, c.Args...)

	return append(args, "-t", formatSeconds(c.Source.Duration), "-f", c.Muxer, outputPath)
}

// sourceArgs returns the lavfi inputs and the stream mapping for a source.
func sourceArgs(source sourceSpec) []string {
	var inputs, outputs []string

	inputIndex := 0
	duration := formatSeconds(source.Duration)

	if source.hasVideo() {
		inputs = append(inputs, "-f", "lavfi", "-i", fmt.Sprintf(
			"testsrc2=size=%dx%d:rate=%d:duration=%s",
			source.Width, source.Height, source.FrameRate, duration))
		outputs = append(outputs, "-map", fmt.Sprintf("%d:v:0", inputIndex))
		inputIndex++
	}

	if source.hasAudio() {
		inputs = append(inputs, "-f", "lavfi", "-i", fmt.Sprintf(
			"sine=frequency=%d:sample_rate=%d:duration=%s",
			sineFrequencyHz, source.SampleRate, duration))
		outputs = append(outputs, "-map", fmt.Sprintf("%d:a:0", inputIndex),
			"-ac", strconv.Itoa(source.Channels))
	}

	return append(inputs, outputs...)
}

// formatSeconds formats a duration the way FFmpeg accepts it, e.g. "2" or "0.5".
func formatSeconds(duration time.Duration) string {
	return strconv.FormatFloat(duration.Seconds(), 'f', -1, 64)
}
