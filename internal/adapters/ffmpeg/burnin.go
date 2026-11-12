package ffmpeg

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// BurnInKind says how a subtitle is drawn into the video.
type BurnInKind string

// The two ways of drawing a subtitle (ADR 0030 §3).
const (
	// BurnInImage overlays a bitmap subtitle stream (PGS, VobSub, DVB) of the input.
	BurnInImage BurnInKind = "image"
	// BurnInText renders a text subtitle (SRT, ASS, ...) with libass.
	BurnInText BurnInKind = "text"
)

const (
	filterComplexFlag = "-filter_complex"
	// burnInLabel names the output pad of the burn-in graph that -map selects.
	burnInLabel = "burnedin"
)

// BurnIn is the subtitle HLSJob draws into the transcoded video. It carries no text of the
// request: the stream is a validated number and every path was chosen by the app layer inside
// a library root or the session directory.
type BurnIn struct {
	Kind BurnInKind
	// StreamIndex is the ffprobe index of the bitmap subtitle stream in the input; BurnInImage
	// only. It addresses the stream as [0:<index>], so the choice never depends on text.
	StreamIndex int
	// File is the absolute path libass reads; BurnInText only. For an embedded track it is the
	// media file itself, for an external one the subtitle file.
	File string
	// Ordinal is the position of the track among the subtitle streams of File (the "si" option
	// of the subtitles filter); BurnInText only, 0 for a file with one track.
	Ordinal int
	// FontsDirectory is an absolute directory with fonts for libass, which is the only source
	// of fonts when the system has none (the Docker image); optional.
	FontsDirectory string
}

// escapeFilterValue makes value one literal option value inside a filtergraph string. It
// applies the two escaping levels of FFmpeg's filtergraph syntax ("Notes on filtergraph
// escaping" in the filter documentation): first the characters that end or quote a filter
// option value, then the ones that structure the graph. Escaping, never quoting, because a
// quote inside a value would end the quoted section.
func escapeFilterValue(value string) string {
	optionLevel := strings.NewReplacer(`\`, `\\`, `'`, `\'`, `:`, `\:`)
	graphLevel := strings.NewReplacer(
		`\`, `\\`, `'`, `\'`, `[`, `\[`, `]`, `\]`, `=`, `\=`, `;`, `\;`, `,`, `\,`)

	return graphLevel.Replace(optionLevel.Replace(value))
}

// checkFilterPath rejects a path that cannot be handed to a filter safely: not absolute, in
// the Windows device namespace, with control characters (a newline would end the option in
// some FFmpeg versions) or with leading or trailing blanks, which the option parser trims.
func checkFilterPath(name, path string) error {
	switch {
	case !filepath.IsAbs(path):
		return fmt.Errorf("burn-in %s %q is not absolute: %w", name, path, domain.ErrInvalid)
	case isWindowsDevicePath(path):
		return fmt.Errorf("burn-in %s in the Windows device namespace: %w", name, domain.ErrInvalid)
	case strings.TrimSpace(path) != path:
		return fmt.Errorf("burn-in %s %q has leading or trailing blanks: %w", name, path, domain.ErrInvalid)
	case strings.ContainsFunc(path, unicode.IsControl):
		return fmt.Errorf("burn-in %s %q contains control characters: %w", name, path, domain.ErrInvalid)
	}

	return nil
}

// validate checks the fields the kind uses.
func (burnIn BurnIn) validate() error {
	switch burnIn.Kind {
	case BurnInImage:
		if burnIn.StreamIndex < 0 {
			return fmt.Errorf("burn-in subtitle stream %d is negative: %w", burnIn.StreamIndex, domain.ErrInvalid)
		}

		return nil
	case BurnInText:
		return burnIn.validateText()
	default:
		return fmt.Errorf("burn-in kind %q: %w", burnIn.Kind, domain.ErrInvalid)
	}
}

func (burnIn BurnIn) validateText() error {
	if burnIn.Ordinal < 0 {
		return fmt.Errorf("burn-in subtitle ordinal %d is negative: %w", burnIn.Ordinal, domain.ErrInvalid)
	}

	if err := checkFilterPath("subtitle file", burnIn.File); err != nil {
		return err
	}

	if burnIn.FontsDirectory == "" {
		return nil
	}

	return checkFilterPath("fonts directory", burnIn.FontsDirectory)
}

// applyBurnIn adds the subtitle to the video transcode of streams: it returns the
// -filter_complex arguments, and re-points the video of streams to the graph output, which
// already contains the scaling the plan asks for. A plan that burns in without a BurnIn, or
// a BurnIn without a re-encoded video, is a caller error: copied video cannot carry an overlay,
// and dropping the subtitle silently would show the user something else than they chose.
//
// A hardware encoder reads device frames, so its upload tail ends the graph and the encoder
// arguments add no -vf of their own (settings.hardware.uploadedInGraph); the software path and
// the software fallback get the graph without the upload.
func (streams *hlsStreams) applyBurnIn(plan playback.Plan, burnIn *BurnIn, settings *encoderSettings) ([]string, error) {
	if burnIn == nil {
		if plan.Subtitle.Mode == playback.SubtitleBurnIn {
			return nil, fmt.Errorf("plan burns subtitle %s in, but the job carries none: %w", plan.Subtitle.TrackID, domain.ErrInvalid)
		}

		return nil, nil
	}

	if streams.video == nil || streams.video.Action != playback.ActionTranscode {
		return nil, fmt.Errorf("burn-in needs a re-encoded video: %w", domain.ErrInvalid)
	}

	if err := burnIn.validate(); err != nil {
		return nil, err
	}

	video := *streams.video

	graph, err := burnInGraph(*burnIn, video, settings.hardware.uploadTail())
	if err != nil {
		return nil, err
	}

	if settings.hardware != nil {
		settings.hardware.uploadedInGraph = true
	}

	// The graph scales; the encoder arguments must not add a second -vf.
	video.ScaleRequired = false
	streams.video = &video
	streams.videoMap = "[" + burnInLabel + "]"

	return []string{filterComplexFlag, graph}, nil
}

// burnInGraph builds the filtergraph that draws burnIn into the video stream of video and
// scales the result to the planned size, ending in the pad burnInLabel.
//
// Image subtitles are overlaid at source size and scaled afterwards, so the bitmap, which
// is drawn for the source canvas, keeps its place on a downscaled picture. Text is rendered
// after scaling, which keeps glyphs sharp.
//
// uploadFilter is appended last. Hardware encoders (M4-09b) read their frames from the GPU,
// but overlay and libass work on software frames, so the graph stays in software and the
// encoder arguments pass "format=nv12,hwupload" (or the API's variant) here.
func burnInGraph(burnIn BurnIn, video playback.StreamDecision, uploadFilter string) (string, error) {
	var scale []string

	if video.ScaleRequired {
		filter, err := scaleFilter(video.TargetWidth, video.TargetHeight)
		if err != nil {
			return "", err
		}

		scale = []string{filter}
	}

	videoPad := "[0:" + strconv.Itoa(video.SourceIndex) + "]"

	var chain []string

	switch burnIn.Kind {
	case BurnInImage:
		chain = append([]string{"overlay"}, scale...)
		chain[0] = videoPad + "[0:" + strconv.Itoa(burnIn.StreamIndex) + "]" + chain[0]
	case BurnInText:
		chain = append(scale, subtitlesFilter(burnIn))
		chain[0] = videoPad + chain[0]
	default:
		return "", fmt.Errorf("burn-in kind %q: %w", burnIn.Kind, domain.ErrInvalid)
	}

	if uploadFilter != "" {
		chain = append(chain, uploadFilter)
	}

	return strings.Join(chain, ",") + "[" + burnInLabel + "]", nil
}

// subtitlesFilter renders the text track with libass.
func subtitlesFilter(burnIn BurnIn) string {
	filter := "subtitles=filename=" + escapeFilterValue(filepath.ToSlash(burnIn.File)) +
		":si=" + strconv.Itoa(burnIn.Ordinal)

	if burnIn.FontsDirectory != "" {
		filter += ":fontsdir=" + escapeFilterValue(filepath.ToSlash(burnIn.FontsDirectory))
	}

	return filter
}

// embeddedStreamIndex returns the ffprobe index of an embedded subtitle track id ("s:<index>")
// and false for any other id.
func embeddedStreamIndex(trackID string) (int, bool) {
	text, found := strings.CutPrefix(trackID, "s:")
	if !found {
		return 0, false
	}

	index, err := strconv.Atoi(text)
	if err != nil || index < 0 {
		return 0, false
	}

	return index, true
}

// burnInFor builds the BurnIn of a session job from the track the plan selected. Image tracks
// of the input are addressed by their ffprobe index (the producer reads the input through the
// descriptor it confined). Text tracks need the stream ordinal and fonts, external tracks a
// confined file path; only the app layer knows them, and until it passes them (BACKLOG
// M4-08b) these are unsupported, so the session fails clearly instead of showing no subtitle.
func burnInFor(track *playback.SubtitleTrack) (*BurnIn, error) {
	if track == nil {
		return nil, nil
	}

	index, embedded := embeddedStreamIndex(track.ID)
	if !embedded || track.External {
		return nil, fmt.Errorf("burning in the external subtitle %s: %w", track.ID, domain.ErrUnsupported)
	}

	if !track.TextBased && isImageCodec(track.Format) {
		return &BurnIn{Kind: BurnInImage, StreamIndex: index}, nil
	}

	return nil, fmt.Errorf("burning in the text subtitle %s: %w", track.ID, domain.ErrUnsupported)
}

// isImageCodec reports a bitmap subtitle codec (ffprobe name).
func isImageCodec(codec string) bool {
	return media.Stream{Codec: codec, Kind: media.StreamKindSubtitle}.IsBitmapSubtitle()
}
