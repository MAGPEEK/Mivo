package corpustest_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/corpustest"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// The transcode targets of milestone 2 (ADR 0023), written out here so the test states the
// promise instead of repeating the engine's constants.
const (
	transcodedVideoCodec   = "h264"
	transcodedVideoProfile = "High"
	transcodedPixelFormat  = "yuv420p"
	transcodedAudioCodec   = "aac"
)

// Files of a session's HLS output (ADR 0024).
const (
	playlistFile     = "index.m3u8"
	initFile         = "init.mp4"
	playlistEndList  = "#EXT-X-ENDLIST"
	initReference    = `#EXT-X-MAP:URI="init.mp4"`
	segmentNameShape = "seg-%05d.m4s"
	joinedFileName   = "joined.mp4"
	joinedFileMode   = 0o600
)

// outputSummary is what ffprobe reports for the joined HLS output of a session.
type outputSummary struct {
	videoCodec   string
	videoProfile string
	pixelFormat  string
	audioCodec   string
	duration     time.Duration
}

// probeOutput reads the finished output of a session through the stream access the
// delivery adapter uses, joins init.mp4 and the segments into one fragmented MP4 and probes
// it. Probing the playlist itself would need options that differ between FFmpeg 7.1 and 9.0.
func (graph *corpusGraph) probeOutput(t *testing.T, service *appplayback.Service, id string) outputSummary {
	t.Helper()

	access, err := service.Access(t.Context(), id)
	if err != nil {
		t.Fatalf("Access: %v", err)
	}

	defer access.Release()

	if access.Delivery != playback.DeliveryHLS || access.Directory == "" {
		t.Fatalf("stream access = %s in %q, want HLS in a session directory", access.Delivery, access.Directory)
	}

	joined := joinOutput(t, access.Directory, finishedSegments(t, access.Directory))

	description, err := graph.prober.Probe(t.Context(), joined)
	if err != nil {
		t.Fatalf("probe the joined output: %v", err)
	}

	summary := outputSummary{duration: description.Duration}

	if video, found := description.PrimaryVideo(); found {
		summary.videoCodec, summary.videoProfile, summary.pixelFormat = video.Codec, video.Profile, video.PixelFormat
	}

	if audio, found := description.DefaultAudio(); found {
		summary.audioCodec = audio.Codec
	}

	return summary
}

// finishedSegments checks the finished playlist (end tag, init segment, relative segment
// names in order) and that the session directory holds exactly the playlist, init.mp4 and
// the listed segments, so no temporary playlist was left behind. It returns the segment
// names in playlist order.
func finishedSegments(t *testing.T, directory string) []string {
	t.Helper()

	playlist, err := os.ReadFile(filepath.Join(directory, playlistFile)) //nolint:gosec // session directory of the test
	if err != nil {
		t.Fatalf("read the playlist: %v", err)
	}

	text := strings.TrimSpace(string(playlist))
	if !strings.HasSuffix(text, playlistEndList) || !strings.Contains(text, initReference) {
		t.Fatalf("the playlist lacks %s or %s:\n%s", playlistEndList, initReference, text)
	}

	var segments []string

	for line := range strings.Lines(text) {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			if want := fmt.Sprintf(segmentNameShape, len(segments)); line != want {
				t.Fatalf("segment %d is listed as %q, want %q", len(segments), line, want)
			}

			segments = append(segments, line)
		}
	}

	if len(segments) == 0 {
		t.Fatalf("the finished playlist lists no segment:\n%s", text)
	}

	assertDirectoryHolds(t, directory, append([]string{playlistFile, initFile}, segments...))

	return segments
}

func assertDirectoryHolds(t *testing.T, directory string, want []string) {
	t.Helper()

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("list the session directory: %v", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	if got := slices.Sorted(slices.Values(names)); !slices.Equal(got, slices.Sorted(slices.Values(want))) {
		t.Errorf("the session directory holds %q, want %q", got, want)
	}
}

// joinOutput writes init.mp4 followed by the segments into one file in a temporary
// directory, which ffprobe reads as a fragmented MP4.
func joinOutput(t *testing.T, directory string, segments []string) string {
	t.Helper()

	var joined bytes.Buffer

	for _, name := range append([]string{initFile}, segments...) {
		data, err := os.ReadFile(filepath.Join(directory, name)) //nolint:gosec // session directory of the test
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}

		joined.Write(data)
	}

	path := filepath.Join(t.TempDir(), joinedFileName)
	if err := os.WriteFile(path, joined.Bytes(), joinedFileMode); err != nil {
		t.Fatalf("write the joined output: %v", err)
	}

	return path
}

// assertOutput checks the probed output against the plan: the target codecs of re-encoded
// streams, the source codecs of copied ones, and the length of the clip after the start.
func assertOutput(t *testing.T, run sessionRun, start time.Duration, output outputSummary) {
	t.Helper()

	wantVideo, wantAudio := run.video.clip.Expected.Video.Codec, run.video.clip.defaultAudioCodec()

	if run.want.Video == playback.ActionTranscode {
		wantVideo = transcodedVideoCodec

		if output.videoProfile != transcodedVideoProfile || output.pixelFormat != transcodedPixelFormat {
			t.Errorf("transcoded video = profile %q, %s; want %s, %s",
				output.videoProfile, output.pixelFormat, transcodedVideoProfile, transcodedPixelFormat)
		}
	}

	if run.want.Audio == playback.ActionTranscode {
		wantAudio = transcodedAudioCodec
	}

	if output.videoCodec != wantVideo || output.audioCodec != wantAudio {
		t.Errorf("output codecs = video %q, audio %q; want video %q, audio %q",
			output.videoCodec, output.audioCodec, wantVideo, wantAudio)
	}

	if wantDuration := run.video.clip.duration() - start; (output.duration - wantDuration).Abs() > corpustest.DurationTolerance {
		t.Errorf("output duration = %s, want %s (±%s): the clip lasts %s and the session starts at %s",
			output.duration, wantDuration, corpustest.DurationTolerance, run.video.clip.duration(), start)
	}
}
