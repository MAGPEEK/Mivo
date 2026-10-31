package ffmpeg_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

const (
	// canaryName is the library file the hostile inputs reference. It is a playable clip, so
	// an FFmpeg that followed the reference would succeed with its streams.
	canaryName = "canary.mkv"
	// notOnWhitelist is FFmpeg's error for a demuxer outside -format_whitelist. FFmpeg checks
	// the whitelist before the demuxer reads its header, which is where concat and hls open
	// the files they list, so the message proves the canary was never opened.
	notOnWhitelist = "not on whitelist"
	// hostileKeyframePosition is any position after the start, so the lookup runs ffprobe.
	hostileKeyframePosition = time.Second
)

// hostileInput is a library file whose content makes FFmpeg open another library file.
type hostileInput struct {
	name     string
	fileName string
	content  string
	// detectedByContent marks content FFmpeg recognises without the file extension, so the
	// producer's inherited descriptor on Linux (/proc/self/fd/3, no extension) selects the
	// same demuxer as the path.
	detectedByContent bool
}

func hostileInputs() []hostileInput {
	playlist := "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:2\n#EXTINF:2.0,\n" + canaryName + "\n#EXT-X-ENDLIST\n"

	return []hostileInput{
		// FFmpeg detects HLS only with an .m3u8 or .m3u extension; it refuses this one on its
		// own, which the test checks as well.
		{name: "HLS playlist named mkv", fileName: "evil.mkv", content: playlist},
		{name: "HLS playlist", fileName: "evil.m3u8", content: playlist},
		{
			name: "ffconcat list named mkv", fileName: "evil.mkv",
			content: "ffconcat version 1.0\nfile " + canaryName + "\n", detectedByContent: true,
		},
	}
}

// TestHostileInputsNeverOpenOtherFiles: a library file whose content is a playlist or a
// concat list must not make probe, keyframe lookup or producer read the file it references
// (ADR 0022 Notes M2-24b). A control run without the demuxer whitelist shows whether the
// local FFmpeg follows the reference at all; when it does, the protected runs must fail with
// FFmpeg's whitelist error.
func TestHostileInputsNeverOpenOtherFiles(t *testing.T) {
	tools, manifest := requireHLSTools(t)
	canary := corpusPath(t, corpusCaseByName(t, manifest, "video-mkv-h264-aac").File)

	for _, input := range hostileInputs() {
		t.Run(input.name, func(t *testing.T) {
			t.Parallel()

			library := t.TempDir()
			copyFile(t, canary, library, canaryName)
			path := filepath.Join(library, input.fileName)

			if err := os.WriteFile(path, []byte(input.content), 0o600); err != nil {
				t.Fatalf("write %s: %v", input.fileName, err)
			}

			followed := followsReference(t, tools.ffprobePath, path)
			t.Logf("an unrestricted ffprobe follows the reference: %t", followed)

			assertProbeRefused(t, tools.prober, path, followed)
			assertKeyframeLookupRefused(t, tools.prober, path, followed)

			producerSeesTheName := runtime.GOOS != "linux"
			assertProductionRefused(t, corpusProducer(t, tools), appplayback.Job{
				SessionID: producerSessionID, Plan: hostilePlan(), LibraryRoot: library,
				RelativePath: input.fileName, InputPath: path, OutputDir: t.TempDir(),
			}, followed && (input.detectedByContent || producerSeesTheName))
		})
	}
}

// followsReference runs ffprobe without the demuxer whitelist and reports whether it read the
// canary's streams through path.
func followsReference(t *testing.T, ffprobePath, path string) bool {
	t.Helper()

	// The executable comes from PATH and the arguments are a slice; no shell is involved.
	control := exec.CommandContext(t.Context(), ffprobePath, //nolint:gosec // see above
		"-v", "quiet", "-protocol_whitelist", "file", "-show_entries", "stream=codec_name",
		"-print_format", "csv", "file:"+path)

	output, err := control.Output()

	return err == nil && strings.Contains(string(output), "h264")
}

func assertProbeRefused(t *testing.T, prober *ffmpeg.Prober, path string, wantWhitelistError bool) {
	t.Helper()

	description, err := prober.Probe(t.Context(), path)
	if !errors.Is(err, domain.ErrUnsupported) {
		t.Fatalf("Probe = %+v, %v, want ErrUnsupported", description, err)
	}

	assertRefusedByFFmpeg(t, "probe", err, wantWhitelistError)
}

func assertKeyframeLookupRefused(t *testing.T, prober *ffmpeg.Prober, path string, wantWhitelistError bool) {
	t.Helper()

	keyframe, err := prober.KeyframeAtOrBefore(t.Context(), path, hostileKeyframePosition)
	if !errors.Is(err, domain.ErrUnsupported) {
		t.Fatalf("KeyframeAtOrBefore = %s, %v, want ErrUnsupported", keyframe, err)
	}

	assertRefusedByFFmpeg(t, "keyframe lookup", err, wantWhitelistError)
}

func assertProductionRefused(t *testing.T, producer *ffmpeg.Producer, job appplayback.Job, wantWhitelistError bool) {
	t.Helper()

	production := startCorpusProduction(t, producer, job)

	ctx, cancel := context.WithTimeout(t.Context(), hlsRunTimeout)
	defer cancel()

	err := production.Ready(ctx)
	if err == nil || errors.Is(err, appplayback.ErrProductionStopped) || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Ready = %v, want FFmpeg's exit error", err)
	}

	assertRefusedByFFmpeg(t, "producer", err, wantWhitelistError)

	if entries, _ := os.ReadDir(job.OutputDir); len(entries) > 0 {
		t.Errorf("the refused production wrote %v", entries)
	}
}

// assertRefusedByFFmpeg checks that err is FFmpeg's own refusal and, when the control run
// followed the reference, that the demuxer whitelist refused it.
func assertRefusedByFFmpeg(t *testing.T, step string, err error, wantWhitelistError bool) {
	t.Helper()

	tail, isExit := ffmpeg.ErrorOutput(err)
	if !isExit {
		t.Fatalf("%s error %v is not FFmpeg's exit error", step, err)
	}

	if wantWhitelistError && !strings.Contains(tail, notOnWhitelist) {
		t.Errorf("%s error output = %q, want FFmpeg's %q", step, tail, notOnWhitelist)
	}
}

// hostilePlan is the remux plan of the canary clip.
func hostilePlan() playback.Plan {
	return playback.Plan{
		Mode: playback.ModeRemux, Delivery: playback.DeliveryHLS,
		SourceContainer: media.ContainerMKV, TargetContainer: media.ContainerMP4,
		Streams: []playback.StreamDecision{
			{Kind: media.StreamKindVideo, SourceIndex: 0, Action: playback.ActionCopy, SourceCodec: "h264", TargetCodec: "h264"},
			{Kind: media.StreamKindAudio, SourceIndex: 1, Action: playback.ActionCopy, SourceCodec: "aac", TargetCodec: "aac"},
		},
	}
}

// TestProbeRefusesImageSequencePatterns: image2 reads frame0.jpg, frame1.jpg, … for a file
// named frame%d.jpg, so a path with "%" must not reach it; a plain image name still probes.
func TestProbeRefusesImageSequencePatterns(t *testing.T) {
	ffprobePath, manifest := requireCorpus(t)
	image := corpusPath(t, corpusCaseByName(t, manifest, "image-jpeg").File)
	prober := ffmpeg.NewProber(ffprobePath)

	library := t.TempDir()
	for _, name := range []string{"frame%d.jpg", "frame0.jpg", "frame1.jpg", "frame2.jpg"} {
		copyFile(t, image, library, name)
	}

	pattern := filepath.Join(library, "frame%d.jpg")

	// The executable comes from PATH and the arguments are a slice; no shell is involved.
	control := exec.CommandContext(t.Context(), ffprobePath, //nolint:gosec // see above
		"-v", "quiet", "-protocol_whitelist", "file", "-count_frames", "-show_entries", "stream=nb_read_frames",
		"-print_format", "csv=print_section=0", "file:"+pattern)
	if output, err := control.Output(); err == nil {
		t.Logf("an unrestricted ffprobe reads %s frames through the pattern", strings.TrimSpace(string(output)))
	}

	_, err := prober.Probe(t.Context(), pattern)
	if !errors.Is(err, domain.ErrUnsupported) {
		t.Fatalf("Probe(frame%%d.jpg) = %v, want ErrUnsupported", err)
	}

	assertRefusedByFFmpeg(t, "probe", err, true)

	if _, err := prober.Probe(t.Context(), filepath.Join(library, "frame1.jpg")); err != nil {
		t.Errorf("Probe(frame1.jpg) = %v, want a description", err)
	}
}
