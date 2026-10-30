package ffmpeg_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/domain/playback/playbacktest"
)

const (
	// hlsRunTimeout bounds one FFmpeg run over a corpus clip (at most 30 s of 320x240).
	hlsRunTimeout = 2 * time.Minute
	// hlsDurationTolerance absorbs the frame and audio-frame granularity at both ends of
	// the produced stream.
	hlsDurationTolerance = 300 * time.Millisecond
	// hlsStartTolerance is how close to 0 the first packet of each stream must be: the output
	// timeline starts at 0 (ADR 0024), shifted by the B-frame reorder delay (80 ms for the
	// corpus clips) and the first audio frame boundary.
	hlsStartTolerance = 150 * time.Millisecond
	// hlsMaxHeight is the transcode height cap of OD-5 B.
	hlsMaxHeight     = 1080
	hlsEndList       = "#EXT-X-ENDLIST"
	hlsInitReference = `#EXT-X-MAP:URI="init.mp4"`
	hlsPlaylistFile  = "index.m3u8"
	hlsInitFile      = "init.mp4"
	// hlsKeyframeFlag marks a keyframe in ffprobe's packet flags ("K__", "K_D", ...).
	hlsKeyframeFlag = "K"
)

// hlsCorpusRun is one corpus clip produced as HLS with the arguments of HLSArguments.
type hlsCorpusRun struct {
	name string
	// asMPEGTS copies the clip into MPEG-TS first. The corpus has no MPEG-TS clip with AAC,
	// whose ADTS frames need a bitstream filter in fMP4, although DVB recordings often are.
	asMPEGTS bool
	// start is the requested position; a copied video starts at the keyframe before it.
	start     time.Duration
	profile   playback.ClientProfile
	wantMode  playback.Mode
	wantVideo playback.Action
	wantAudio playback.Action
}

// hevcHLSProfile is the Chromium profile plus HEVC inside HLS (like Safari or Edge with
// hardware decoding), so HEVC is copied and tagged hvc1.
func hevcHLSProfile() playback.ClientProfile {
	profile := playbacktest.ChromiumDesktop()
	profile.HLS.VideoCodecs = append(profile.HLS.VideoCodecs, "hevc")

	return profile
}

// TestHLSArgumentsProduceTheirPlanWithFFmpeg runs the produced argument lists through the
// local FFmpeg and checks the result with ffprobe: a finished fMP4 HLS stream with relative
// URIs, the planned codecs and size, a keyframe at the start of every segment and an output
// timeline that starts at 0, also after a seek.
func TestHLSArgumentsProduceTheirPlanWithFFmpeg(t *testing.T) {
	tools, manifest := requireHLSTools(t)
	chromium := playbacktest.ChromiumDesktop()

	runs := []hlsCorpusRun{
		{
			name: "video-mkv-h264-aac", profile: chromium, wantMode: playback.ModeRemux,
			wantVideo: playback.ActionCopy, wantAudio: playback.ActionCopy,
		},
		{
			name: "video-avi-xvid-mp3", profile: chromium, wantMode: playback.ModeTranscode,
			wantVideo: playback.ActionTranscode, wantAudio: playback.ActionCopy,
		},
		{
			name: "video-m2ts-h264-ac3", profile: chromium, wantMode: playback.ModeTranscode,
			wantVideo: playback.ActionCopy, wantAudio: playback.ActionTranscode,
		},
		{
			name: "video-mkv-h264-aac-long", start: 13 * time.Second, profile: chromium,
			wantMode: playback.ModeRemux, wantVideo: playback.ActionCopy, wantAudio: playback.ActionCopy,
		},
		{
			name: "video-mkv-h264-aac-long", asMPEGTS: true, profile: chromium,
			wantMode: playback.ModeRemux, wantVideo: playback.ActionCopy, wantAudio: playback.ActionCopy,
		},
		{
			name: "video-mkv-h264-aac-long", asMPEGTS: true, start: 21 * time.Second, profile: chromium,
			wantMode: playback.ModeRemux, wantVideo: playback.ActionCopy, wantAudio: playback.ActionCopy,
		},
		{
			name: "video-ts-mpeg2-mp2-long", start: 13*time.Second + 300*time.Millisecond, profile: chromium,
			wantMode: playback.ModeTranscode, wantVideo: playback.ActionTranscode, wantAudio: playback.ActionTranscode,
		},
		{
			name: "video-mkv-hevc10-ac3", profile: hevcHLSProfile(), wantMode: playback.ModeTranscode,
			wantVideo: playback.ActionCopy, wantAudio: playback.ActionTranscode,
		},
		{
			name: "audio-flac-24bit-96k", profile: chromium, wantMode: playback.ModeRemux,
			wantAudio: playback.ActionCopy,
		},
	}

	for _, run := range runs {
		t.Run(hlsRunName(run), func(t *testing.T) {
			t.Parallel()

			source := corpusPath(t, corpusCaseByName(t, manifest, run.name).File)
			if run.asMPEGTS {
				source = hlsCopyToMPEGTS(t, tools.ffmpegPath, source)
			}

			description, plan := hlsDecide(t, tools.prober, source, run)
			start := tools.startPosition(t, source, plan, run.start)
			directory := t.TempDir()

			hlsRunFFmpeg(t, tools.ffmpegPath, ffmpeg.HLSJob{
				Plan: plan, InputPath: source, StartPosition: start, OutputDirectory: directory,
			})

			segments := hlsAssertPlaylist(t, directory)
			tools.assertOutput(t, hlsJoin(t, directory, segments...), plan, description.Duration-start)
			tools.assertSegmentStarts(t, directory, segments, plan)
		})
	}
}

func hlsRunName(run hlsCorpusRun) string {
	name := run.name
	if run.asMPEGTS {
		name += " as mpegts"
	}

	if run.start > 0 {
		name += " from " + run.start.String()
	}

	return name
}

// hlsCopyToMPEGTS copies every stream of source into an MPEG-TS file in a temporary
// directory.
func hlsCopyToMPEGTS(t *testing.T, ffmpegPath, source string) string {
	t.Helper()

	target := filepath.Join(t.TempDir(), "clip.ts")

	// The executable comes from PATH and the arguments are a slice; no shell is involved.
	command := exec.CommandContext(t.Context(), ffmpegPath, //nolint:gosec // see above
		"-hide_banner", "-nostdin", "-loglevel", "error", "-protocol_whitelist", "file",
		"-i", "file:"+source, "-map", "0", "-c", "copy", "-f", "mpegts", "file:"+target)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("copy %s into MPEG-TS: %v\n%s", filepath.Base(source), err, output)
	}

	return target
}

// hlsTools are the local FFmpeg executables the integration test runs and checks with.
type hlsTools struct {
	ffmpegPath  string
	ffprobePath string
	prober      *ffmpeg.Prober
}

// requireHLSTools returns the local FFmpeg tools and the corpus manifest, or skips.
func requireHLSTools(t *testing.T) (hlsTools, corpusManifest) {
	t.Helper()

	ffprobePath, manifest := requireCorpus(t)

	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("skipping FFmpeg HLS integration test: ffmpeg is not on PATH " +
			"(install FFmpeg, see docs/development.md)")
	}

	return hlsTools{ffmpegPath: ffmpegPath, ffprobePath: ffprobePath, prober: ffmpeg.NewProber(ffprobePath)}, manifest
}

// hlsDecide probes the clip and decides it like the playback service does.
func hlsDecide(t *testing.T, prober *ffmpeg.Prober, source string, run hlsCorpusRun) (media.Description, playback.Plan) {
	t.Helper()

	description, err := prober.Probe(t.Context(), source)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}

	plan, err := playback.Decide(playback.Input{
		Description:  description,
		Profile:      run.profile,
		Capabilities: playbacktest.FullCapabilities(),
		Request:      playback.Request{StartPosition: run.start},
		Policy:       playback.Policy{MaxTranscodeHeight: hlsMaxHeight},
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}

	video, _ := plan.Video()
	audio, _ := plan.Audio()

	if plan.Mode != run.wantMode || video.Action != run.wantVideo || audio.Action != run.wantAudio {
		t.Fatalf("plan = %s (video %q, audio %q), want %s (video %q, audio %q)",
			plan.Mode, video.Action, audio.Action, run.wantMode, run.wantVideo, run.wantAudio)
	}

	return description, plan
}

// startPosition moves a seek with copied video to the keyframe before it (ADR 0024).
func (tools hlsTools) startPosition(t *testing.T, source string, plan playback.Plan, requested time.Duration) time.Duration {
	t.Helper()

	video, hasVideo := plan.Video()
	if requested == 0 || !hasVideo || video.Action != playback.ActionCopy {
		return requested
	}

	keyframe, err := tools.prober.KeyframeAtOrBefore(t.Context(), source, requested)
	if err != nil {
		t.Fatalf("KeyframeAtOrBefore(%s): %v", requested, err)
	}

	if keyframe == 0 {
		t.Fatalf("no keyframe before %s; the seek clip should have one every 2 s", requested)
	}

	return keyframe
}

func hlsRunFFmpeg(t *testing.T, ffmpegPath string, job ffmpeg.HLSJob) {
	t.Helper()

	arguments, err := ffmpeg.HLSArguments(job)
	if err != nil {
		t.Fatalf("HLSArguments: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), hlsRunTimeout)
	defer cancel()

	// The executable comes from PATH and the arguments are a slice; no shell is involved.
	command := exec.CommandContext(ctx, ffmpegPath, arguments...) //nolint:gosec // see above
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg %q: %v\n%s", arguments, err, output)
	}
}

// hlsAssertPlaylist checks the finished EVENT playlist and that the directory holds exactly
// its files (no temporary files left, nothing outside the session directory), and returns
// the segment names in playlist order.
func hlsAssertPlaylist(t *testing.T, directory string) []string {
	t.Helper()

	playlist, err := os.ReadFile(filepath.Join(directory, hlsPlaylistFile)) //nolint:gosec // test directory
	if err != nil {
		t.Fatalf("read playlist: %v", err)
	}

	text := strings.TrimSpace(string(playlist))
	if !strings.HasSuffix(text, hlsEndList) || !strings.Contains(text, hlsInitReference) {
		t.Fatalf("playlist lacks %s or %s:\n%s", hlsEndList, hlsInitReference, text)
	}

	var segments []string

	for line := range strings.Lines(text) {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			segments = append(segments, line)
		}
	}

	for index, segment := range segments {
		if want := fmt.Sprintf("seg-%05d.m4s", index); segment != want {
			t.Fatalf("segment %d is referenced as %q, want the relative name %q", index, segment, want)
		}
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("list output directory: %v", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	if want := append([]string{hlsPlaylistFile, hlsInitFile}, segments...); !hlsSameNames(names, want) {
		t.Fatalf("output directory holds %q, want %q", names, want)
	}

	return segments
}

func hlsSameNames(got, want []string) bool {
	return slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(want)))
}

// hlsJoin writes init.mp4 followed by the segments into one file, which players and
// ffprobe read as a fragmented MP4.
func hlsJoin(t *testing.T, directory string, segments ...string) string {
	t.Helper()

	var joined bytes.Buffer

	for _, name := range append([]string{hlsInitFile}, segments...) {
		data, err := os.ReadFile(filepath.Join(directory, name)) //nolint:gosec // test directory
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}

		joined.Write(data)
	}

	path := filepath.Join(t.TempDir(), "joined.mp4")
	if err := os.WriteFile(path, joined.Bytes(), 0o600); err != nil {
		t.Fatalf("write joined stream: %v", err)
	}

	return path
}

// assertOutput probes the joined stream: the planned codecs, size and channels, and the
// length of the source after the start position.
func (tools hlsTools) assertOutput(t *testing.T, joined string, plan playback.Plan, wantDuration time.Duration) {
	t.Helper()

	output, err := tools.prober.Probe(t.Context(), joined)
	if err != nil {
		t.Fatalf("Probe(joined stream): %v", err)
	}

	if (output.Duration - wantDuration).Abs() > hlsDurationTolerance {
		t.Errorf("output duration = %s, want %s", output.Duration, wantDuration)
	}

	if want, ok := plan.Video(); ok {
		hlsAssertVideo(t, output, want)
	} else if _, found := output.PrimaryVideo(); found {
		t.Error("output has a video stream, the plan has none")
	}

	if want, ok := plan.Audio(); ok {
		hlsAssertAudio(t, output, want)
	} else if _, found := output.DefaultAudio(); found {
		t.Error("output has an audio stream, the plan has none")
	}
}

func hlsAssertVideo(t *testing.T, output media.Description, want playback.StreamDecision) {
	t.Helper()

	video, ok := output.PrimaryVideo()
	if !ok {
		t.Fatal("output has no video stream")
	}

	if video.Codec != want.TargetCodec || video.Width != want.TargetWidth || video.Height != want.TargetHeight {
		t.Errorf("output video = %s %dx%d, want %s %dx%d",
			video.Codec, video.Width, video.Height, want.TargetCodec, want.TargetWidth, want.TargetHeight)
	}

	if video.Width%2 != 0 || video.Height%2 != 0 {
		t.Errorf("output size %dx%d is not even", video.Width, video.Height)
	}

	if want.Action == playback.ActionTranscode && (video.Profile != "High" || video.PixelFormat != "yuv420p") {
		t.Errorf("transcoded video = profile %q, %s; want High, yuv420p", video.Profile, video.PixelFormat)
	}

	if want.TargetCodec == "hevc" && video.CodecTag != "hvc1" {
		t.Errorf("copied HEVC has the tag %q, want hvc1", video.CodecTag)
	}
}

func hlsAssertAudio(t *testing.T, output media.Description, want playback.StreamDecision) {
	t.Helper()

	audio, ok := output.DefaultAudio()
	if !ok {
		t.Fatal("output has no audio stream")
	}

	if audio.Codec != want.TargetCodec || audio.Channels != want.TargetChannels {
		t.Errorf("output audio = %s with %d channels, want %s with %d",
			audio.Codec, audio.Channels, want.TargetCodec, want.TargetChannels)
	}
}

// assertSegmentStarts checks that every segment begins with a video keyframe, so a
// player can start at any segment, and that the output timeline begins at 0 for every
// stream: a stream that starts later (or a copy that began at an earlier keyframe) would
// shift the picture against the position the session reports (ADR 0024).
func (tools hlsTools) assertSegmentStarts(t *testing.T, directory string, segments []string, plan playback.Plan) {
	t.Helper()

	first := hlsJoin(t, directory, segments[0])

	if _, hasAudio := plan.Audio(); hasAudio {
		if timestamp, _ := hlsFirstPacket(t, tools.ffprobePath, first, "a:0"); timestamp < 0 || timestamp > hlsStartTolerance {
			t.Errorf("audio starts at %s, want 0", timestamp)
		}
	}

	if _, hasVideo := plan.Video(); !hasVideo {
		return
	}

	for index, segment := range segments {
		timestamp, flags := hlsFirstPacket(t, tools.ffprobePath, hlsJoin(t, directory, segment), "v:0")

		if !strings.Contains(flags, hlsKeyframeFlag) {
			t.Errorf("%s starts with a packet flagged %q, want a keyframe", segment, flags)
		}

		if index == 0 && (timestamp < 0 || timestamp > hlsStartTolerance) {
			t.Errorf("video starts at %s, want 0", timestamp)
		}
	}
}

// hlsFirstPacket returns the presentation time and the flags of the first packet of the
// selected stream of an fMP4 file.
func hlsFirstPacket(t *testing.T, ffprobePath, path, streamSelector string) (time.Duration, string) {
	t.Helper()

	// The executable comes from PATH and the arguments are a slice; no shell is involved.
	command := exec.CommandContext(t.Context(), ffprobePath, //nolint:gosec // see above
		"-v", "error", "-protocol_whitelist", "file", "-select_streams", streamSelector,
		"-show_entries", "packet=pts_time,flags", "-read_intervals", "%+#1",
		"-print_format", "csv=print_section=0", "file:"+path)

	output, err := command.Output()
	if err != nil {
		t.Fatalf("ffprobe packets of %s: %v", filepath.Base(path), err)
	}

	line, _, _ := strings.Cut(strings.TrimSpace(string(output)), "\n")
	fields := strings.Split(strings.TrimSpace(line), ",")

	if len(fields) < 2 {
		t.Fatalf("ffprobe printed no %s packet for %s: %q", streamSelector, filepath.Base(path), output)
	}

	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		t.Fatalf("packet time %q: %v", fields[0], err)
	}

	return time.Duration(seconds * float64(time.Second)), fields[1]
}
