package ffmpeg_test

import (
	"errors"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/domain/playback/playbacktest"
)

// The expected arguments below are written out literally on purpose: they are the contract
// with FFmpeg (ADR 0024) and must not follow the implementation's constants.

// hlsSingleFileDemuxers is the -format_whitelist of every input (ADR 0022 Notes M2-24b);
// image2 joins it for paths without "%".
const (
	hlsSingleFileDemuxers = "matroska,mov,mpegts,mpeg,mpegvideo,avi,flv,live_flv,ogg,asf,mxf,rm," +
		"flac,mp3,wav,aiff,wv,ape,dsf,iff,aac,ac3,eac3,dts," +
		"gif,png_pipe,jpeg_pipe,tiff_pipe,webp_pipe," +
		"srt,webvtt,ass,sup"
	hlsDemuxersWithImages = hlsSingleFileDemuxers + ",image2"
)

func hlsCopyVideo(index int, codec string) playback.StreamDecision {
	return playback.StreamDecision{
		Kind: media.StreamKindVideo, SourceIndex: index, Action: playback.ActionCopy,
		SourceCodec: codec, TargetCodec: codec, TargetWidth: 320, TargetHeight: 240,
	}
}

func hlsCopyAudio(index int, codec string) playback.StreamDecision {
	return playback.StreamDecision{
		Kind: media.StreamKindAudio, SourceIndex: index, Action: playback.ActionCopy,
		SourceCodec: codec, TargetCodec: codec, TargetChannels: 2,
	}
}

func hlsTranscodeVideo(index, width, height int, scaleRequired bool) playback.StreamDecision {
	return playback.StreamDecision{
		Kind: media.StreamKindVideo, SourceIndex: index, Action: playback.ActionTranscode,
		SourceCodec: "mpeg4", TargetCodec: "h264", TargetWidth: width, TargetHeight: height,
		ScaleRequired: scaleRequired, Encoder: "libx264",
	}
}

func hlsTranscodeAudio(index int) playback.StreamDecision {
	return playback.StreamDecision{
		Kind: media.StreamKindAudio, SourceIndex: index, Action: playback.ActionTranscode,
		SourceCodec: "ac3", TargetCodec: "aac", TargetChannels: 2, Encoder: "aac",
	}
}

func hlsPlan(mode playback.Mode, streams ...playback.StreamDecision) playback.Plan {
	return playback.Plan{
		Mode: mode, Delivery: playback.DeliveryHLS,
		SourceContainer: media.ContainerMKV, TargetContainer: media.ContainerMP4,
		Streams: streams,
	}
}

// hlsAbsolutePath returns an absolute path on the current OS (C:\... on Windows).
func hlsAbsolutePath(t *testing.T, elements ...string) string {
	t.Helper()

	path, err := filepath.Abs(filepath.Join(append([]string{string(filepath.Separator)}, elements...)...))
	if err != nil {
		t.Fatalf("absolute path: %v", err)
	}

	return path
}

func hlsLibx264(extra ...string) []string {
	return append([]string{
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-profile:v", "high",
		"-pix_fmt", "yuv420p", "-force_key_frames", "expr:gte(t,n_forced*6)", "-sc_threshold", "0",
	}, extra...)
}

func hlsAAC() []string {
	return []string{"-c:a", "aac", "-ac", "2", "-b:a", "192k"}
}

// hlsShape holds the parts of an argument list that differ per plan: the seek before the
// input, the stream maps and the codec options.
type hlsShape struct {
	seek, maps, codecs []string
}

// hlsExpected assembles a complete argument list.
func hlsExpected(input, outputDirectory string, shape hlsShape) []string {
	output := "file:" + filepath.ToSlash(outputDirectory)

	expected := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y"}
	expected = append(expected, shape.seek...)
	expected = append(expected, "-format_whitelist", hlsDemuxersWithImages, "-protocol_whitelist", "file", "-i", "file:"+input)
	expected = append(expected, shape.maps...)
	expected = append(expected, "-sn", "-dn", "-map_metadata", "-1", "-map_chapters", "-1")
	expected = append(expected, shape.codecs...)

	return append(expected,
		"-f", "hls", "-hls_time", "6", "-hls_playlist_type", "event",
		"-hls_segment_type", "fmp4", "-hls_fmp4_init_filename", "init.mp4",
		"-hls_flags", "independent_segments+temp_file", "-start_number", "0",
		"-hls_segment_filename", output+"/seg-%05d.m4s", output+"/index.m3u8")
}

func TestHLSArgumentsPerPlanShape(t *testing.T) {
	t.Parallel()

	input := hlsAbsolutePath(t, "media", "Film (2024)", "film.mkv")
	output := hlsAbsolutePath(t, "transcode", "sessions", "4f1c2d0e-5b6a-4c7d-8e9f-0a1b2c3d4e5f")
	bothMaps := []string{"-map", "0:0", "-map", "0:1"}

	tests := []struct {
		name   string
		plan   playback.Plan
		start  time.Duration
		seek   []string
		maps   []string
		codecs []string
	}{
		{
			name: "remux copies both streams",
			plan: hlsPlan(playback.ModeRemux, hlsCopyVideo(0, "h264"), hlsCopyAudio(1, "aac")),
			maps: bothMaps, codecs: []string{"-c:v", "copy", "-c:a", "copy", "-bsf:a", "aac_adtstoasc"},
		},
		{
			name: "video-only transcode copies the audio",
			plan: hlsPlan(playback.ModeTranscode, hlsTranscodeVideo(0, 320, 240, false), hlsCopyAudio(1, "mp3")),
			maps: bothMaps, codecs: append(hlsLibx264(), "-c:a", "copy"),
		},
		{
			name: "audio-only transcode copies the video",
			plan: hlsPlan(playback.ModeTranscode, hlsCopyVideo(0, "h264"), hlsTranscodeAudio(1)),
			maps: bothMaps, codecs: append([]string{"-c:v", "copy"}, hlsAAC()...),
		},
		{
			name: "transcode of both streams",
			plan: hlsPlan(playback.ModeTranscode, hlsTranscodeVideo(0, 320, 240, false), hlsTranscodeAudio(1)),
			maps: bothMaps, codecs: append(hlsLibx264(), hlsAAC()...),
		},
		{
			name: "downscale to the height cap",
			plan: hlsPlan(playback.ModeTranscode, hlsTranscodeVideo(0, 1920, 1080, true), hlsCopyAudio(1, "aac")),
			maps: bothMaps, codecs: append(hlsLibx264("-vf", "scale=1920:1080"), "-c:a", "copy", "-bsf:a", "aac_adtstoasc"),
		},
		{
			name: "downscale with unknown source width keeps the aspect ratio",
			plan: hlsPlan(playback.ModeTranscode, hlsTranscodeVideo(0, 0, 1080, true), hlsCopyAudio(1, "aac")),
			maps: bothMaps, codecs: append(hlsLibx264("-vf", "scale=-2:1080"), "-c:a", "copy", "-bsf:a", "aac_adtstoasc"),
		},
		{
			name: "odd dimensions are rounded down without a downscale (720x405 to 720x404)",
			plan: hlsPlan(playback.ModeTranscode, hlsTranscodeVideo(0, 720, 404, true), hlsCopyAudio(1, "aac")),
			maps: bothMaps, codecs: append(hlsLibx264("-vf", "scale=720:404"), "-c:a", "copy", "-bsf:a", "aac_adtstoasc"),
		},
		{
			name:  "seek start goes before the input",
			plan:  hlsPlan(playback.ModeRemux, hlsCopyVideo(0, "h264"), hlsCopyAudio(1, "aac")),
			start: 12*time.Second + 500*time.Millisecond, seek: []string{"-ss", "12.500000"},
			maps: bothMaps, codecs: []string{"-c:v", "copy", "-c:a", "copy", "-bsf:a", "aac_adtstoasc", "-copypriorss", "0"},
		},
		{
			name:  "sub-millisecond seek start keeps microseconds",
			plan:  hlsPlan(playback.ModeTranscode, hlsTranscodeVideo(0, 320, 240, false), hlsCopyAudio(1, "aac")),
			start: 3*time.Minute + 7*time.Microsecond, seek: []string{"-ss", "180.000007"},
			maps: bothMaps, codecs: append(hlsLibx264(), "-c:a", "copy", "-bsf:a", "aac_adtstoasc", "-copypriorss", "0"),
		},
		{
			name: "file without audio maps the video only",
			plan: hlsPlan(playback.ModeRemux, hlsCopyVideo(0, "h264")),
			maps: []string{"-map", "0:0"}, codecs: []string{"-c:v", "copy"},
		},
		{
			name: "file without video maps the audio only",
			plan: hlsPlan(playback.ModeRemux, hlsCopyAudio(0, "flac")),
			maps: []string{"-map", "0:0"}, codecs: []string{"-c:a", "copy"},
		},
		{
			name: "HEVC copy is tagged hvc1",
			plan: hlsPlan(playback.ModeRemux, hlsCopyVideo(0, "hevc"), hlsCopyAudio(1, "aac")),
			maps: bothMaps, codecs: []string{"-c:v", "copy", "-tag:v", "hvc1", "-c:a", "copy", "-bsf:a", "aac_adtstoasc"},
		},
		{
			name: "selected streams are mapped by their source index, video first",
			plan: hlsPlan(playback.ModeRemux, hlsCopyAudio(3, "aac"), hlsCopyVideo(1, "h264")),
			maps: []string{"-map", "0:1", "-map", "0:3"}, codecs: []string{"-c:v", "copy", "-c:a", "copy", "-bsf:a", "aac_adtstoasc"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := ffmpeg.HLSArguments(ffmpeg.HLSJob{
				Plan: test.plan, InputPath: input, StartPosition: test.start, OutputDirectory: output,
			})
			if err != nil {
				t.Fatalf("HLSArguments: %v", err)
			}

			want := hlsExpected(input, output, hlsShape{seek: test.seek, maps: test.maps, codecs: test.codecs})
			if !slices.Equal(got, want) {
				t.Errorf("HLSArguments =\n%q\nwant\n%q", got, want)
			}
		})
	}
}

func TestHLSArgumentsPassesTheWhitelistsBeforeTheInput(t *testing.T) {
	t.Parallel()

	got, err := ffmpeg.HLSArguments(ffmpeg.HLSJob{
		Plan:            hlsPlan(playback.ModeRemux, hlsCopyVideo(0, "h264")),
		InputPath:       hlsAbsolutePath(t, "media", "100% Wolf.mkv"),
		StartPosition:   time.Second,
		OutputDirectory: hlsAbsolutePath(t, "transcode", "session"),
	})
	if err != nil {
		t.Fatalf("HLSArguments: %v", err)
	}

	input := slices.Index(got, "-i")

	// Input options apply to the next -i only, so both whitelists must precede it. A path
	// with "%" could be an image2 sequence pattern, so image2 is not allowed for it.
	for option, value := range map[string]string{"-format_whitelist": hlsSingleFileDemuxers, "-protocol_whitelist": "file"} {
		whitelist := slices.Index(got, option)
		if whitelist < 0 || whitelist+1 >= len(got) || got[whitelist+1] != value {
			t.Fatalf("arguments %q lack %s %s", got, option, value)
		}

		if input < whitelist {
			t.Errorf("%s at %d must precede -i at %d: %q", option, whitelist, input, got)
		}
	}

	if seek := slices.Index(got, "-ss"); seek < 0 || seek > input {
		t.Errorf("-ss at %d must precede -i at %d: %q", seek, input, got)
	}

	if slices.Contains(got, "-copyts") {
		t.Errorf("arguments %q contain -copyts; the output must start at 0 (ADR 0024)", got)
	}
}

// A file name that looks like an option or a protocol stays one argument behind file:.
func TestHLSArgumentsKeepsHostileNamesInsideOneArgument(t *testing.T) {
	t.Parallel()

	input := hlsAbsolutePath(t, "media", "-i concat:x.mkv|y.mkv; rm -rf ~.mkv")

	got, err := ffmpeg.HLSArguments(ffmpeg.HLSJob{
		Plan:            hlsPlan(playback.ModeRemux, hlsCopyVideo(0, "h264")),
		InputPath:       input,
		OutputDirectory: hlsAbsolutePath(t, "transcode", "session"),
	})
	if err != nil {
		t.Fatalf("HLSArguments: %v", err)
	}

	if index := slices.Index(got, "-i"); index < 0 || got[index+1] != "file:"+input {
		t.Errorf("arguments %q do not pass the input as one file: URL", got)
	}
}

func TestHLSArgumentsWritesOutputPathsWithForwardSlashes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		directory string
		wantBase  string
	}{
		{
			name: "trailing separator", directory: hlsAbsolutePath(t, "transcode") + string(filepath.Separator),
			wantBase: "file:" + filepath.ToSlash(hlsAbsolutePath(t, "transcode")),
		},
		// The root keeps its separator (C:\ or /); it must not be doubled.
		{
			name: "file system root", directory: hlsAbsolutePath(t),
			wantBase: "file:" + filepath.ToSlash(hlsAbsolutePath(t)),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := ffmpeg.HLSArguments(ffmpeg.HLSJob{
				Plan:            hlsPlan(playback.ModeRemux, hlsCopyVideo(0, "h264")),
				InputPath:       hlsAbsolutePath(t, "media", "film.mkv"),
				OutputDirectory: test.directory,
			})
			if err != nil {
				t.Fatalf("HLSArguments: %v", err)
			}

			base := test.wantBase
			if len(base) > 0 && base[len(base)-1] == '/' {
				base = base[:len(base)-1]
			}

			if want := base + "/index.m3u8"; got[len(got)-1] != want {
				t.Errorf("playlist = %q, want %q", got[len(got)-1], want)
			}

			if want := base + "/seg-%05d.m4s"; got[len(got)-2] != want {
				t.Errorf("segment template = %q, want %q", got[len(got)-2], want)
			}
		})
	}
}

// The decision engine sets ScaleRequired for odd sizes and the height cap; the arguments
// must carry the filter it asks for (M2-09 review).
func TestHLSArgumentsScaleTheSizeTheEngineDecided(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		width, height int
		maxHeight     int
		wantFilter    string
	}{
		{name: "odd height", width: 720, height: 405, wantFilter: "scale=720:404"},
		{name: "odd width", width: 721, height: 400, maxHeight: 1080, wantFilter: "scale=720:400"},
		{name: "height cap", width: 3840, height: 2160, maxHeight: 1080, wantFilter: "scale=1920:1080"},
		{name: "even size within the cap", width: 1280, height: 720, maxHeight: 1080},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			plan := hlsDecideMPEG4(t, test.width, test.height, test.maxHeight)

			got, err := ffmpeg.HLSArguments(ffmpeg.HLSJob{
				Plan: plan, InputPath: hlsAbsolutePath(t, "media", "film.avi"),
				OutputDirectory: hlsAbsolutePath(t, "transcode", "session"),
			})
			if err != nil {
				t.Fatalf("HLSArguments: %v", err)
			}

			filter := ""
			if index := slices.Index(got, "-vf"); index >= 0 {
				filter = got[index+1]
			}

			if filter != test.wantFilter {
				t.Errorf("video filter = %q, want %q (arguments %q)", filter, test.wantFilter, got)
			}
		})
	}
}

// hlsDecideMPEG4 decides an AVI with MPEG-4 Part 2 video, which every browser profile
// transcodes.
func hlsDecideMPEG4(t *testing.T, width, height, maxHeight int) playback.Plan {
	t.Helper()

	plan, err := playback.Decide(playback.Input{
		Description: media.Description{
			Container: media.ContainerAVI, Duration: time.Minute,
			Streams: []media.Stream{
				{Index: 0, Kind: media.StreamKindVideo, Codec: "mpeg4", Width: width, Height: height},
				{Index: 1, Kind: media.StreamKindAudio, Codec: "mp3", Channels: 2},
			},
		},
		Profile:      playbacktest.ChromiumDesktop(),
		Capabilities: playbacktest.FullCapabilities(),
		Policy:       playback.Policy{MaxTranscodeHeight: maxHeight},
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}

	return plan
}

func TestHLSArgumentsRejectsJobsItCannotProduce(t *testing.T) {
	t.Parallel()

	input := hlsAbsolutePath(t, "media", "film.mkv")
	output := hlsAbsolutePath(t, "transcode", "session")
	remux := hlsPlan(playback.ModeRemux, hlsCopyVideo(0, "h264"), hlsCopyAudio(1, "aac"))

	withEncoder := func(decision playback.StreamDecision, encoder, target string) playback.StreamDecision {
		decision.Encoder, decision.TargetCodec = encoder, target

		return decision
	}

	tests := []struct {
		name string
		job  ffmpeg.HLSJob
		want error
	}{
		{name: "direct play plan", want: domain.ErrInvalid, job: ffmpeg.HLSJob{
			Plan: playback.Plan{
				Mode: playback.ModeDirectPlay, Delivery: playback.DeliveryFile,
				Streams: []playback.StreamDecision{hlsCopyVideo(0, "h264")},
			},
			InputPath: input, OutputDirectory: output,
		}},
		{name: "unknown mode", want: domain.ErrInvalid, job: ffmpeg.HLSJob{
			Plan: hlsPlan("", hlsCopyVideo(0, "h264")), InputPath: input, OutputDirectory: output,
		}},
		{name: "empty input path", want: domain.ErrInvalid, job: ffmpeg.HLSJob{
			Plan: remux, OutputDirectory: output,
		}},
		{name: "relative input path", want: domain.ErrInvalid, job: ffmpeg.HLSJob{
			Plan: remux, InputPath: filepath.Join("media", "film.mkv"), OutputDirectory: output,
		}},
		{name: "empty output directory", want: domain.ErrInvalid, job: ffmpeg.HLSJob{
			Plan: remux, InputPath: input,
		}},
		{name: "relative output directory", want: domain.ErrInvalid, job: ffmpeg.HLSJob{
			Plan: remux, InputPath: input, OutputDirectory: filepath.Join("transcode", "session"),
		}},
		{name: "percent sign in the output directory", want: domain.ErrInvalid, job: ffmpeg.HLSJob{
			Plan: remux, InputPath: input, OutputDirectory: hlsAbsolutePath(t, "100%", "session"),
		}},
		{name: "negative start", want: domain.ErrInvalid, job: ffmpeg.HLSJob{
			Plan: remux, InputPath: input, OutputDirectory: output, StartPosition: -time.Second,
		}},
		{name: "no streams", want: domain.ErrInvalid, job: ffmpeg.HLSJob{
			Plan: hlsPlan(playback.ModeRemux), InputPath: input, OutputDirectory: output,
		}},
		{name: "two video streams", want: domain.ErrInvalid, job: ffmpeg.HLSJob{
			Plan:      hlsPlan(playback.ModeRemux, hlsCopyVideo(0, "h264"), hlsCopyVideo(1, "h264")),
			InputPath: input, OutputDirectory: output,
		}},
		{name: "two audio streams", want: domain.ErrInvalid, job: ffmpeg.HLSJob{
			Plan:      hlsPlan(playback.ModeRemux, hlsCopyAudio(0, "aac"), hlsCopyAudio(1, "aac")),
			InputPath: input, OutputDirectory: output,
		}},
		{name: "subtitle stream", want: domain.ErrInvalid, job: ffmpeg.HLSJob{
			Plan: hlsPlan(playback.ModeRemux, hlsCopyVideo(0, "h264"), playback.StreamDecision{
				Kind: media.StreamKindSubtitle, SourceIndex: 2, Action: playback.ActionCopy,
			}),
			InputPath: input, OutputDirectory: output,
		}},
		{name: "negative source index", want: domain.ErrInvalid, job: ffmpeg.HLSJob{
			Plan: hlsPlan(playback.ModeRemux, hlsCopyVideo(-1, "h264")), InputPath: input, OutputDirectory: output,
		}},
		{name: "unknown action", want: domain.ErrInvalid, job: ffmpeg.HLSJob{
			Plan: hlsPlan(playback.ModeRemux, playback.StreamDecision{
				Kind: media.StreamKindAudio, SourceIndex: 1, Action: "burn",
			}),
			InputPath: input, OutputDirectory: output,
		}},
		{name: "remux plan that re-encodes", want: domain.ErrInvalid, job: ffmpeg.HLSJob{
			Plan:      hlsPlan(playback.ModeRemux, hlsCopyVideo(0, "h264"), hlsTranscodeAudio(1)),
			InputPath: input, OutputDirectory: output,
		}},
		{name: "transcode plan that only copies", want: domain.ErrInvalid, job: ffmpeg.HLSJob{
			Plan:      hlsPlan(playback.ModeTranscode, hlsCopyVideo(0, "h264"), hlsCopyAudio(1, "aac")),
			InputPath: input, OutputDirectory: output,
		}},
		{name: "scale without a size", want: domain.ErrInvalid, job: ffmpeg.HLSJob{
			Plan:      hlsPlan(playback.ModeTranscode, hlsTranscodeVideo(0, 0, 0, true)),
			InputPath: input, OutputDirectory: output,
		}},
		{name: "scale to a negative size", want: domain.ErrInvalid, job: ffmpeg.HLSJob{
			Plan:      hlsPlan(playback.ModeTranscode, hlsTranscodeVideo(0, -2, 720, true)),
			InputPath: input, OutputDirectory: output,
		}},
		{name: "audio transcode without channels", want: domain.ErrInvalid, job: ffmpeg.HLSJob{
			Plan: hlsPlan(playback.ModeTranscode, func() playback.StreamDecision {
				decision := hlsTranscodeAudio(1)
				decision.TargetChannels = 0

				return decision
			}()),
			InputPath: input, OutputDirectory: output,
		}},
		{name: "hardware video encoder", want: domain.ErrUnsupported, job: ffmpeg.HLSJob{
			Plan:      hlsPlan(playback.ModeTranscode, withEncoder(hlsTranscodeVideo(0, 320, 240, false), "h264_nvenc", "h264")),
			InputPath: input, OutputDirectory: output,
		}},
		{name: "video target other than H.264", want: domain.ErrUnsupported, job: ffmpeg.HLSJob{
			Plan:      hlsPlan(playback.ModeTranscode, withEncoder(hlsTranscodeVideo(0, 320, 240, false), "libx264", "hevc")),
			InputPath: input, OutputDirectory: output,
		}},
		{name: "video transcode without encoder", want: domain.ErrUnsupported, job: ffmpeg.HLSJob{
			Plan:      hlsPlan(playback.ModeTranscode, withEncoder(hlsTranscodeVideo(0, 320, 240, false), "", "h264")),
			InputPath: input, OutputDirectory: output,
		}},
		{name: "other audio encoder", want: domain.ErrUnsupported, job: ffmpeg.HLSJob{
			Plan:      hlsPlan(playback.ModeTranscode, withEncoder(hlsTranscodeAudio(1), "libfdk_aac", "aac")),
			InputPath: input, OutputDirectory: output,
		}},
		{name: "audio target other than AAC", want: domain.ErrUnsupported, job: ffmpeg.HLSJob{
			Plan:      hlsPlan(playback.ModeTranscode, withEncoder(hlsTranscodeAudio(1), "aac", "opus")),
			InputPath: input, OutputDirectory: output,
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := ffmpeg.HLSArguments(test.job)
			if !errors.Is(err, test.want) {
				t.Fatalf("HLSArguments error = %v, want %v", err, test.want)
			}

			if got != nil {
				t.Errorf("HLSArguments returned arguments %q together with an error", got)
			}
		})
	}
}

func TestHLSArgumentsRejectsWindowsDeviceOutputDirectories(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "windows" {
		t.Skip("Windows device paths exist on Windows only")
	}

	for _, directory := range []string{`\\.\pipe\session`, `\\?\GLOBALROOT\Device\HarddiskVolume1\session`} {
		_, err := ffmpeg.HLSArguments(ffmpeg.HLSJob{
			Plan:            hlsPlan(playback.ModeRemux, hlsCopyVideo(0, "h264")),
			InputPath:       hlsAbsolutePath(t, "media", "film.mkv"),
			OutputDirectory: directory,
		})
		if !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("HLSArguments(%s) error = %v, want ErrInvalid", directory, err)
		}
	}
}
