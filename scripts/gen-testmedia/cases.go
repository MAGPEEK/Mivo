package main

import (
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"time"
)

// Categories group cases in the output directory and in case names.
const (
	categoryVideo      = "video"
	categoryAudio      = "audio"
	categoryImage      = "image"
	categorySubtitle   = "subtitle"
	categoryMultiTrack = "multitrack"
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

// gapReasonPrefix starts the reason of every known gap, so readers find the explanation.
const gapReasonPrefix = "known gap (ADR 0021): "

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
	// Inputs are text files the generator writes to a temporary directory and passes to
	// FFmpeg after the lavfi inputs, e.g. subtitles to embed or a chapter list.
	Inputs []textInput
	// Sidecars are text files written next to Output once it was built.
	Sidecars []sidecarFile
	// Args are the encoder options of the case. They are placed after the generic
	// reproducibility options, so a case can override those.
	Args []string
	// Encoders are FFmpeg encoder names (as listed by `ffmpeg -encoders`) the case needs.
	Encoders []string
	// Muxer is the FFmpeg muxer name passed to -f; it is required as well.
	Muxer  string
	Expect expectation
	// Gap, when set, marks a format FFmpeg cannot produce at all (ADR 0021). The case is never
	// built and is listed as skipped with this reason, so the corpus shows what is missing.
	Gap string
}

// sourceSpec describes the lavfi inputs of a case. A zero Width means no video input and a
// zero SampleRate means no audio input.
type sourceSpec struct {
	Width      int
	Height     int
	FrameRate  int
	SampleRate int
	Channels   int
	// AudioTracks is the number of audio streams; values below one mean one. Each track is a
	// sine tone of its own frequency, so players can tell the tracks apart by ear.
	AudioTracks int
	// Frames, when positive, limits the output to that many video frames. Such outputs are
	// images and have no expected duration.
	Frames   int
	Duration time.Duration
}

// inputRole says how a text input is used in the output.
type inputRole int

const (
	// roleSubtitle maps the input's subtitle stream into the output.
	roleSubtitle inputRole = iota
	// roleChapters copies the input's chapters (an FFmetadata file) into the output.
	roleChapters
)

// textInput is a text file the generator writes before FFmpeg runs.
type textInput struct {
	// FileName is the base name inside the temporary input directory.
	FileName string
	// Format is the FFmpeg demuxer passed to -f, e.g. "srt", "ass" or "ffmetadata".
	Format  string
	Role    inputRole
	Content string
}

// sidecarFile is a text file next to the media file, such as an external subtitle.
type sidecarFile struct {
	// Output is the path relative to the output directory, with forward slashes.
	Output  string
	Content string
	// ProbeFormat and Codec are what ffprobe reports for the file.
	ProbeFormat string
	Codec       string
}

// expectation lists the properties ffprobe should report for the generated file. Duration,
// dimensions, sample rate and channel count are derived from the source when the manifest
// is written. Zero values mean "not checked", which is what known gaps use.
type expectation struct {
	// Container is the short container name Mivo uses, e.g. "mkv", "m2ts" or "webm".
	Container string `json:"container"`
	// ProbeFormat is the format_name ffprobe reports, e.g. "matroska,webm".
	ProbeFormat string `json:"probeFormat"`
	// DurationSeconds is the nominal length; images have none.
	DurationSeconds float64               `json:"durationSeconds,omitempty"`
	Video           *videoExpectation     `json:"video,omitempty"`
	Audio           []audioExpectation    `json:"audio,omitempty"`
	Subtitles       []subtitleExpectation `json:"subtitles,omitempty"`
	Chapters        []chapterExpectation  `json:"chapters,omitempty"`
	// Tags are container-level metadata tags such as title, artist and album.
	Tags     map[string]string    `json:"tags,omitempty"`
	Sidecars []sidecarExpectation `json:"sidecars,omitempty"`
}

// videoExpectation describes the expected video stream. Codec names use ffprobe's codec_name.
type videoExpectation struct {
	Codec          string `json:"codec"`
	Profile        string `json:"profile,omitempty"`
	CodecTag       string `json:"codecTag,omitempty"`
	PixelFormat    string `json:"pixelFormat,omitempty"`
	ColorPrimaries string `json:"colorPrimaries,omitempty"`
	ColorTransfer  string `json:"colorTransfer,omitempty"`
	// ColorSpace is the colour matrix ffprobe reports as color_space, e.g. "bt2020nc".
	ColorSpace string `json:"colorSpace,omitempty"`
	// HDR names the HDR format whose metadata the stream carries, e.g. "hdr10".
	HDR    string `json:"hdr,omitempty"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

// audioExpectation describes one expected audio stream, in stream order.
type audioExpectation struct {
	Codec      string `json:"codec"`
	SampleRate int    `json:"sampleRate,omitempty"`
	Channels   int    `json:"channels,omitempty"`
	// BitDepth is the bits_per_raw_sample of lossless codecs, e.g. 24 for 24-bit FLAC.
	BitDepth int `json:"bitDepth,omitempty"`
	// Track is set when the case controls the language tag and the dispositions.
	Track *trackExpectation `json:"track,omitempty"`
}

// subtitleExpectation describes one expected subtitle stream, in stream order.
type subtitleExpectation struct {
	Codec string            `json:"codec"`
	Track *trackExpectation `json:"track,omitempty"`
}

// trackExpectation is the language tag and the dispositions of a stream. The case sets the
// same values through trackArgs, so the expectation and the FFmpeg options cannot diverge.
type trackExpectation struct {
	// Language is an ISO 639-2 code such as "eng" or "deu", as Matroska and players expect.
	Language string `json:"language"`
	Default  bool   `json:"default"`
	Forced   bool   `json:"forced"`
}

// chapterExpectation describes one expected chapter.
type chapterExpectation struct {
	Title        string  `json:"title"`
	StartSeconds float64 `json:"startSeconds"`
	EndSeconds   float64 `json:"endSeconds"`
}

// sidecarExpectation describes a text file written next to the media file.
type sidecarExpectation struct {
	File        string `json:"file"`
	ProbeFormat string `json:"probeFormat"`
	Codec       string `json:"codec"`
}

// allCases is the registry of every case category. A new category adds one line here.
func allCases() []mediaCase {
	return slices.Concat(
		videoCases(),
		audioCases(),
		imageCases(),
		subtitleCases(),
		multiTrackCases(),
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

// audioTrackCount is the number of audio streams the source produces.
func (s sourceSpec) audioTrackCount() int {
	if !s.hasAudio() {
		return 0
	}

	return max(s.AudioTracks, 1)
}

// inputCount is the number of lavfi inputs, which precede the text inputs.
func (s sourceSpec) inputCount() int {
	if s.hasVideo() {
		return 1 + s.audioTrackCount()
	}

	return s.audioTrackCount()
}

// isGap reports whether the case is a known gap that is never built.
func (c mediaCase) isGap() bool { return c.Gap != "" }

// expected returns the case expectation completed with the properties of its source.
func (c mediaCase) expected() expectation {
	result := c.Expect

	if c.Source.Frames == 0 {
		result.DurationSeconds = c.Source.Duration.Seconds()
	}

	if result.Video != nil {
		video := *result.Video
		video.Width = c.Source.Width
		video.Height = c.Source.Height
		result.Video = &video
	}

	result.Audio = slices.Clone(result.Audio)
	for index := range result.Audio {
		result.Audio[index].SampleRate = c.Source.SampleRate
		result.Audio[index].Channels = c.Source.Channels
	}

	for _, sidecar := range c.Sidecars {
		result.Sidecars = append(result.Sidecars, sidecarExpectation{
			File: sidecar.Output, ProbeFormat: sidecar.ProbeFormat, Codec: sidecar.Codec,
		})
	}

	return result
}

// buildArgs returns the complete FFmpeg argument list that writes the case to outputPath.
// inputDir is the directory the text inputs of the case were written to.
func buildArgs(c mediaCase, inputDir, outputPath string) []string {
	sourceInputs, sourceMaps := sourceArgs(c.Source)
	textInputs, textMaps := textInputArgs(c.Inputs, inputDir, c.Source.inputCount())

	return slices.Concat(
		[]string{
			"-hide_banner", "-nostdin", "-loglevel", "error",
			// The generator always writes to a temporary file it owns and renames it
			// afterwards, so overwriting never touches a finished corpus file.
			"-y",
		},
		sourceInputs, textInputs, sourceMaps, textMaps,
		// Strip global and stream metadata and encoder version strings so repeated runs give
		// identical files. A plain "-map_metadata -1" would also drop chapter titles, which
		// only come from inputs the generator writes itself.
		[]string{
			"-map_metadata:g", "-1", "-map_metadata:s", "-1",
			"-fflags", "+bitexact", "-flags", "+bitexact",
		},
		c.Args,
		[]string{"-t", formatSeconds(c.Source.Duration), "-f", c.Muxer, outputPath},
	)
}

// sourceArgs returns the lavfi inputs of a source and the options that map them.
func sourceArgs(source sourceSpec) (inputs, outputs []string) {
	inputIndex := 0
	duration := formatSeconds(source.Duration)

	if source.hasVideo() {
		inputs = append(inputs, "-f", "lavfi", "-i", fmt.Sprintf(
			"testsrc2=size=%dx%d:rate=%d:duration=%s",
			source.Width, source.Height, source.FrameRate, duration))
		outputs = append(outputs, "-map", fmt.Sprintf("%d:v:0", inputIndex))
		inputIndex++
	}

	if source.Frames > 0 {
		outputs = append(outputs, "-frames:v", strconv.Itoa(source.Frames))
	}

	for track := range source.audioTrackCount() {
		inputs = append(inputs, "-f", "lavfi", "-i", fmt.Sprintf(
			"sine=frequency=%d:sample_rate=%d:duration=%s",
			sineFrequencyHz*(track+1), source.SampleRate, duration))
		outputs = append(outputs, "-map", fmt.Sprintf("%d:a:0", inputIndex))
		inputIndex++
	}

	if source.hasAudio() {
		outputs = append(outputs, "-ac", strconv.Itoa(source.Channels))
	}

	return inputs, outputs
}

// textInputArgs returns the inputs for the text files in inputDir and the options that map
// them. firstIndex is the FFmpeg input index of the first text input.
func textInputArgs(textInputs []textInput, inputDir string, firstIndex int) (inputs, outputs []string) {
	for offset, input := range textInputs {
		index := strconv.Itoa(firstIndex + offset)
		inputs = append(inputs, "-f", input.Format, "-i", filepath.Join(inputDir, input.FileName))

		switch input.Role {
		case roleSubtitle:
			outputs = append(outputs, "-map", index+":s:0")
		case roleChapters:
			outputs = append(outputs, "-map_chapters", index)
		}
	}

	return inputs, outputs
}

// trackArgs returns the options that set the language and dispositions of one output
// stream; stream is a stream specifier such as "a:0" or "s:1".
func trackArgs(stream string, track trackExpectation) []string {
	disposition := "0"

	switch {
	case track.Default && track.Forced:
		disposition = "default+forced"
	case track.Default:
		disposition = "default"
	case track.Forced:
		disposition = "forced"
	}

	return []string{"-metadata:s:" + stream, "language=" + track.Language, "-disposition:" + stream, disposition}
}

// knownGap declares a format FFmpeg cannot produce (ADR 0021). The category is the directory
// of output; reason completes the sentence "known gap (ADR 0021): ...".
func knownGap(name, output string, expected expectation, reason string) mediaCase {
	return mediaCase{
		Name: name, Category: path.Dir(output), Output: output, Expect: expected,
		Gap: gapReasonPrefix + reason,
	}
}

// formatSeconds formats a duration the way FFmpeg accepts it, e.g. "2" or "0.5".
func formatSeconds(duration time.Duration) string {
	return strconv.FormatFloat(duration.Seconds(), 'f', -1, 64)
}
