package ffmpeg

import (
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// HLS output of ADR 0024: an EVENT playlist with fMP4 segments of hlsSegmentSeconds in the
// session directory. The file names are the ones the stream handler accepts.
const (
	hlsSegmentSeconds  = 6
	hlsPlaylistName    = "index.m3u8"
	hlsInitName        = "init.mp4"
	hlsSegmentTemplate = "seg-%05d.m4s"
	// hlsFlags: every segment starts with a keyframe, and FFmpeg writes each file under a
	// temporary name and renames it, so the server never serves half a file.
	hlsFlags              = "independent_segments+temp_file"
	hlsFirstSegmentNumber = "0"
	// outputSeparator joins the output directory and the file names. FFmpeg's HLS muxer
	// derives the directory of init.mp4 from the last "/" of the playlist name only; with
	// Windows separators it writes init.mp4 into its working directory instead.
	outputSeparator = "/"
	// templateMarker starts a placeholder in the segment name template (and "%v" in the
	// playlist name), so it must not occur in the output directory.
	templateMarker = "%"
)

// Software transcode targets of milestone 2 (ADR 0023): H.264 High 8-bit 4:2:0 with libx264
// and AAC LC stereo with FFmpeg's native encoder.
const (
	x264Preset = "veryfast"
	// x264ConstantRateFactor is libx264's default quality; the bit rate follows the content
	// until bandwidth tiers arrive in milestone 4.
	x264ConstantRateFactor = "23"
	x264Profile            = "high"
	transcodePixelFormat   = "yuv420p"
	// disabledSceneCutThreshold stops libx264 from inserting extra keyframes at scene cuts,
	// so the forced keyframes alone decide where segments start.
	disabledSceneCutThreshold = "0"
	aacBitRate                = "192k"
	// scaleKeepAspect makes FFmpeg derive an unknown dimension from the other one, keeping
	// the aspect ratio and rounding to an even number. A 0 would mean "the input size".
	scaleKeepAspect = -2
)

// Stream copy settings.
const (
	copyCodec = "copy"
	// appleHEVCTag: Safari plays HEVC in MP4 only with the hvc1 sample entry; hev1 (the
	// default for some sources) is rejected.
	appleHEVCTag = "hvc1"
	hevcCodec    = "hevc"
	// aacCodec copied from MPEG-TS arrives as ADTS frames. The mp4 muxer converts them on
	// its own, but not inside FFmpeg's HLS muxer, which fails with "Malformed AAC bitstream"
	// (FFmpeg 9.0.2). The filter passes AAC without ADTS headers through unchanged.
	aacCodec           = "aac"
	adtsToMP4Bitstream = "aac_adtstoasc"
	// dropBeforeStart discards copied packets that lie before -ss. FFmpeg's seek does not
	// land on the keyframe at -ss in every container: Matroska and MPEG-TS with B-frames
	// land up to a GOP earlier (the seek target is moved back by 3/23 s), MPEG-TS even on a
	// packet that is no keyframe. Without the option the copy then starts at an earlier
	// keyframe, or audio starts seconds before the video, and the output no longer begins
	// at the start the session reports (ADR 0024).
	dropBeforeStart = "0"
	// disabledMapping drops the global metadata and the chapters: the player shows Mivo's
	// own metadata, and chapters would only enlarge init.mp4.
	disabledMapping = "-1"
)

// HLSJob is one HLS session HLSArguments describes.
type HLSJob struct {
	// Plan is a remux or transcode plan from playback.Decide.
	Plan playback.Plan
	// InputPath is the absolute path of the media file, resolved inside a library root.
	InputPath string
	// StartPosition is where the output begins, measured from the format start time like
	// FFmpeg's -ss; 0 starts at the beginning. When the video is copied it must be a
	// keyframe (Prober.KeyframeAtOrBefore): copied packets before it are dropped, so any
	// other position starts the video only at the next keyframe.
	StartPosition time.Duration
	// OutputDirectory is the absolute, existing session directory FFmpeg writes index.m3u8,
	// init.mp4 and seg-NNNNN.m4s into. It must not contain "%".
	OutputDirectory string
}

// HLSArguments returns the complete ffmpeg argument list for job (ADR 0024): it reads the
// input from StartPosition, copies or encodes the selected streams as the plan says and
// writes an fMP4 HLS stream into the output directory. The output timeline starts at 0.
//
// Errors wrap domain.ErrInvalid for a job that breaks a rule (a direct-play or inconsistent
// plan, empty or relative paths, a negative start) and domain.ErrUnsupported for a
// transcode target or encoder this adapter does not produce.
func HLSArguments(job HLSJob) ([]string, error) {
	streams, err := selectHLSStreams(job.Plan)
	if err != nil {
		return nil, fmt.Errorf("hls arguments: %w", err)
	}

	encoding, err := streams.encodingArguments()
	if err != nil {
		return nil, fmt.Errorf("hls arguments: %w", err)
	}

	if job.StartPosition < 0 {
		return nil, fmt.Errorf("hls arguments: negative start position %s: %w", job.StartPosition, domain.ErrInvalid)
	}

	input, err := ffmpegInputArguments(job.InputPath)
	if err != nil {
		return nil, fmt.Errorf("hls arguments: %w", err)
	}

	output, err := hlsOutputArguments(job.OutputDirectory)
	if err != nil {
		return nil, fmt.Errorf("hls arguments: %w", err)
	}

	arguments := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y"}

	// -ss before -i seeks the input; without -copyts the output starts at 0 (ADR 0024).
	if job.StartPosition > 0 {
		arguments = append(arguments, "-ss", formatSeconds(job.StartPosition))
	}

	arguments = append(arguments, input...)
	arguments = append(arguments, streams.mapArguments()...)
	arguments = append(arguments, "-sn", "-dn", "-map_metadata", disabledMapping, "-map_chapters", disabledMapping)
	arguments = append(arguments, encoding...)

	if job.StartPosition > 0 {
		arguments = append(arguments, "-copypriorss", dropBeforeStart)
	}

	return append(arguments, output...), nil
}

// ffmpegInputArguments is inputArguments for ffmpeg, which needs -i before the input URL
// where ffprobe takes it as a positional argument.
func ffmpegInputArguments(absolutePath string) ([]string, error) {
	input, err := inputArguments(absolutePath)
	if err != nil {
		return nil, err
	}

	return slices.Insert(input, len(input)-1, "-i"), nil
}

// hlsOutputArguments selects the HLS muxer and names its files inside directory.
func hlsOutputArguments(directory string) ([]string, error) {
	if !filepath.IsAbs(directory) {
		return nil, fmt.Errorf("output directory %q is not absolute: %w", directory, domain.ErrInvalid)
	}

	if isWindowsDevicePath(directory) {
		return nil, fmt.Errorf("output directory in the Windows device namespace: %w", domain.ErrInvalid)
	}

	if strings.Contains(directory, templateMarker) {
		return nil, fmt.Errorf("output directory contains %q, which FFmpeg reads as a name template: %w",
			templateMarker, domain.ErrInvalid)
	}

	// A root directory keeps its separator after Clean ("C:\", "/"); trimming it avoids "//".
	base := fileProtocol + strings.TrimSuffix(filepath.ToSlash(filepath.Clean(directory)), outputSeparator)

	return []string{
		"-f", "hls",
		"-hls_time", strconv.Itoa(hlsSegmentSeconds),
		"-hls_playlist_type", "event",
		"-hls_segment_type", "fmp4",
		"-hls_fmp4_init_filename", hlsInitName,
		"-hls_flags", hlsFlags,
		"-start_number", hlsFirstSegmentNumber,
		"-hls_segment_filename", base + outputSeparator + hlsSegmentTemplate,
		base + outputSeparator + hlsPlaylistName,
	}, nil
}

// hlsStreams holds the selected streams of an HLS plan; nil when the file lacks the kind.
type hlsStreams struct {
	video *playback.StreamDecision
	audio *playback.StreamDecision
}

// selectHLSStreams checks that plan is an HLS plan whose mode matches its stream actions
// and that it selects at most one video and one audio stream.
func selectHLSStreams(plan playback.Plan) (hlsStreams, error) {
	if plan.Mode != playback.ModeRemux && plan.Mode != playback.ModeTranscode {
		return hlsStreams{}, fmt.Errorf("mode %q is not produced as HLS: %w", plan.Mode, domain.ErrInvalid)
	}

	var (
		streams    hlsStreams
		transcodes bool
	)

	for _, decision := range plan.Streams {
		if err := streams.add(decision); err != nil {
			return hlsStreams{}, err
		}

		transcodes = transcodes || decision.Action == playback.ActionTranscode
	}

	if streams.video == nil && streams.audio == nil {
		return hlsStreams{}, fmt.Errorf("plan selects no stream: %w", domain.ErrInvalid)
	}

	if transcodes != (plan.Mode == playback.ModeTranscode) {
		return hlsStreams{}, fmt.Errorf("mode %s does not match the stream actions: %w", plan.Mode, domain.ErrInvalid)
	}

	return streams, nil
}

// add stores decision under its kind.
func (streams *hlsStreams) add(decision playback.StreamDecision) error {
	if decision.SourceIndex < 0 {
		return fmt.Errorf("negative stream index %d: %w", decision.SourceIndex, domain.ErrInvalid)
	}

	if decision.Action != playback.ActionCopy && decision.Action != playback.ActionTranscode {
		return fmt.Errorf("stream %d: unknown action %q: %w", decision.SourceIndex, decision.Action, domain.ErrInvalid)
	}

	slot := &streams.audio
	if decision.Kind == media.StreamKindVideo {
		slot = &streams.video
	} else if decision.Kind != media.StreamKindAudio {
		return fmt.Errorf("stream %d: %s streams are not mapped: %w", decision.SourceIndex, decision.Kind, domain.ErrInvalid)
	}

	if *slot != nil {
		return fmt.Errorf("plan selects more than one %s stream: %w", decision.Kind, domain.ErrInvalid)
	}

	*slot = &decision

	return nil
}

// mapArguments maps the selected streams by their ffprobe index, video first.
func (streams hlsStreams) mapArguments() []string {
	var arguments []string

	for _, decision := range []*playback.StreamDecision{streams.video, streams.audio} {
		if decision != nil {
			arguments = append(arguments, "-map", "0:"+strconv.Itoa(decision.SourceIndex))
		}
	}

	return arguments
}

// encodingArguments returns the codec options of the selected streams, video first.
func (streams hlsStreams) encodingArguments() ([]string, error) {
	var arguments []string

	if streams.video != nil {
		video, err := videoArguments(*streams.video)
		if err != nil {
			return nil, err
		}

		arguments = append(arguments, video...)
	}

	if streams.audio != nil {
		audio, err := audioArguments(*streams.audio)
		if err != nil {
			return nil, err
		}

		arguments = append(arguments, audio...)
	}

	return arguments, nil
}

// videoArguments copies the video or encodes it to the H.264 target with keyframes at every
// segment boundary, so each segment starts with one.
func videoArguments(decision playback.StreamDecision) ([]string, error) {
	if decision.Action == playback.ActionCopy {
		if decision.TargetCodec == hevcCodec {
			return []string{"-c:v", copyCodec, "-tag:v", appleHEVCTag}, nil
		}

		return []string{"-c:v", copyCodec}, nil
	}

	if decision.Encoder != playback.EncoderLibx264 || decision.TargetCodec != playback.TranscodeVideoCodec {
		return nil, fmt.Errorf("video: no arguments for encoder %q to %q: %w",
			decision.Encoder, decision.TargetCodec, domain.ErrUnsupported)
	}

	arguments := []string{
		"-c:v", playback.EncoderLibx264, "-preset", x264Preset, "-crf", x264ConstantRateFactor,
		"-profile:v", x264Profile, "-pix_fmt", transcodePixelFormat,
		"-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", hlsSegmentSeconds),
		"-sc_threshold", disabledSceneCutThreshold,
	}

	if !decision.ScaleRequired {
		return arguments, nil
	}

	filter, err := scaleFilter(decision.TargetWidth, decision.TargetHeight)
	if err != nil {
		return nil, err
	}

	return append(arguments, "-vf", filter), nil
}

// scaleFilter scales to width x height; a 0 dimension (unknown source width) is derived
// from the other one.
func scaleFilter(width, height int) (string, error) {
	if width < 0 || height < 0 || (width == 0 && height == 0) {
		return "", fmt.Errorf("video: cannot scale to %dx%d: %w", width, height, domain.ErrInvalid)
	}

	return "scale=" + scaleDimension(width) + ":" + scaleDimension(height), nil
}

func scaleDimension(dimension int) string {
	if dimension == 0 {
		return strconv.Itoa(scaleKeepAspect)
	}

	return strconv.Itoa(dimension)
}

// audioArguments copies the audio or encodes it to AAC with the planned channel count.
func audioArguments(decision playback.StreamDecision) ([]string, error) {
	if decision.Action == playback.ActionCopy {
		if decision.TargetCodec == aacCodec {
			return []string{"-c:a", copyCodec, "-bsf:a", adtsToMP4Bitstream}, nil
		}

		return []string{"-c:a", copyCodec}, nil
	}

	if decision.Encoder != playback.EncoderAAC || decision.TargetCodec != playback.TranscodeAudioCodec {
		return nil, fmt.Errorf("audio: no arguments for encoder %q to %q: %w",
			decision.Encoder, decision.TargetCodec, domain.ErrUnsupported)
	}

	if decision.TargetChannels <= 0 {
		return nil, fmt.Errorf("audio: invalid channel count %d: %w", decision.TargetChannels, domain.ErrInvalid)
	}

	return []string{
		"-c:a", playback.EncoderAAC, "-ac", strconv.Itoa(decision.TargetChannels), "-b:a", aacBitRate,
	}, nil
}
