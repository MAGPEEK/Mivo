package ffmpeg_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/domain/playback/playbacktest"
)

const (
	// capClipSeconds is the length of the 1080p clip of the bit-rate cap test.
	capClipSeconds = 20
	// capBitsPerSecond is the quality step under test.
	capBitsPerSecond = 4_000_000
	// capTolerance is how far the finished stream may exceed the cap (acceptance criterion).
	capTolerance = 1.3
	bitsPerByte  = 8
)

// TestHLSArgumentsKeepAStreamUnderItsBitRateCap generates a busy 1080p clip, transcodes it at
// a 4 Mbit/s cap and checks the total size of the segments against the cap.
func TestHLSArgumentsKeepAStreamUnderItsBitRateCap(t *testing.T) {
	tools, _ := requireHLSTools(t)
	source := generateCapClip(t, tools.ffmpegPath)

	description, err := tools.prober.Probe(t.Context(), source)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}

	plan, err := playback.Decide(playback.Input{
		Description:  description,
		Profile:      playbacktest.ChromiumDesktop(),
		Capabilities: playbacktest.FullCapabilities(),
		Request:      playback.Request{MaxBitrate: capBitsPerSecond},
		Policy:       playback.Policy{MaxTranscodeHeight: hlsMaxHeight},
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}

	if video, _ := plan.Video(); video.Action != playback.ActionTranscode || video.MaxBitrate != capBitsPerSecond {
		t.Fatalf("video = %+v, want a transcode with the cap", video)
	}

	directory := t.TempDir()
	hlsRunFFmpeg(t, tools.ffmpegPath, ffmpeg.HLSJob{Plan: plan, InputPath: source, OutputDirectory: directory})

	segments := hlsAssertPlaylist(t, directory)
	tools.assertSegmentStarts(t, directory, segments, plan)

	var total int64

	for _, segment := range segments {
		info, err := os.Stat(filepath.Join(directory, segment))
		if err != nil {
			t.Fatalf("stat %s: %v", segment, err)
		}

		total += info.Size()
	}

	limit := float64(capBitsPerSecond) / bitsPerByte * capClipSeconds * capTolerance
	if float64(total) > limit {
		t.Errorf("segments total %d bytes, want at most %.0f (1.3 x the %d bit/s cap over %d s)",
			total, limit, capBitsPerSecond, capClipSeconds)
	}

	t.Logf("segments total %d bytes of %.0f allowed", total, limit)
}

// generateCapClip writes a noisy 20 s 1080p H.264 clip (far above 4 Mbit/s, so the cap binds)
// with stereo AAC into a temporary directory.
func generateCapClip(t *testing.T, ffmpegPath string) string {
	t.Helper()

	requireFFmpegFeature(t, ffmpegPath, ffmpegFeature{listing: "-devices", name: "lavfi", reason: "the fixture is generated with the lavfi input device"})

	target := filepath.Join(t.TempDir(), "cap-1080p.mkv")

	// The executable comes from PATH and the arguments are a slice; no shell is involved.
	command := exec.CommandContext(t.Context(), ffmpegPath, //nolint:gosec // see above
		"-hide_banner", "-nostdin", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=1920x1080:rate=25:duration=20,noise=alls=40:allf=t",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=20",
		"-c:v", "libx264", "-preset", "ultrafast", "-crf", "12", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "128k", "-t", "20", "-f", "matroska", "file:"+target)

	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generate the 1080p clip: %v\n%s", err, output)
	}

	return target
}
