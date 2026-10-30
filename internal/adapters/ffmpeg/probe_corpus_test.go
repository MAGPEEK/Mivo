package ffmpeg_test

import (
	"runtime"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/domain/media"
)

// durationTolerance allows for container framing around the nominal clip length; MPEG-TS
// clips report about 10 ms more than their nominal length.
const durationTolerance = 100 * time.Millisecond

func TestProbeCorpusMatchesManifest(t *testing.T) {
	ffprobePath, manifest := requireCorpus(t)
	prober := ffmpeg.NewProber(ffprobePath)
	t.Logf("corpus generated with FFmpeg %s", manifest.FFmpegVersion)

	probed := 0

	for _, entry := range manifest.Cases {
		if entry.Status != statusGenerated {
			continue
		}

		probed++

		t.Run(entry.Name, func(t *testing.T) {
			t.Parallel()

			description, err := prober.Probe(t.Context(), corpusPath(t, entry.File))
			if err != nil {
				t.Fatalf("Probe(%s): %v", entry.File, err)
			}

			assertFormat(t, description, entry.Expected)
			assertVideo(t, description, entry.Expected.Video)
			assertAudio(t, description, entry.Expected.Audio)
			assertSubtitles(t, description, entry.Expected.Subtitles)
			assertSidecars(t, prober, entry.Expected.Sidecars)
		})
	}

	if probed == 0 {
		t.Fatal("the corpus manifest lists no generated case")
	}
}

// assertFormat cross-checks media.IdentifyContainer against the manifest. Audio and image
// containers (flac, m4a, png, ...) are no Mivo video containers, so only the probe format is
// compared for them.
func assertFormat(t *testing.T, description media.Description, expected corpusExpectation) {
	t.Helper()

	if description.FormatName != expected.ProbeFormat {
		t.Errorf("format name = %q, want %q", description.FormatName, expected.ProbeFormat)
	}

	if container, err := media.ParseContainer(expected.Container); err == nil && description.Container != container {
		t.Errorf("container = %q, want %q", description.Container, container)
	}

	if expected.DurationSeconds > 0 {
		want := time.Duration(expected.DurationSeconds * float64(time.Second))
		if difference := description.Duration - want; difference.Abs() > durationTolerance {
			t.Errorf("duration = %s, want %s ± %s", description.Duration, want, durationTolerance)
		}
	}

	if err := description.Validate(); err != nil {
		t.Errorf("description does not validate: %v", err)
	}
}

func assertVideo(t *testing.T, description media.Description, expected *videoExpectation) {
	t.Helper()

	video, found := description.PrimaryVideo()
	if expected == nil {
		if found {
			t.Errorf("unexpected video stream %+v", video)
		}

		return
	}

	if !found {
		t.Fatal("no video stream")
	}

	checks := []struct{ name, got, want string }{
		{"codec", video.Codec, expected.Codec},
		{"profile", video.Profile, expected.Profile},
		{"codec tag", video.CodecTag, expected.CodecTag},
		{"pixel format", video.PixelFormat, expected.PixelFormat},
		{"color primaries", video.ColorPrimaries, expected.ColorPrimaries},
		{"color transfer", video.ColorTransfer, expected.ColorTransfer},
		{"color space", video.ColorSpace, expected.ColorSpace},
	}
	for _, check := range checks {
		if check.want != "" && check.got != check.want {
			t.Errorf("video %s = %q, want %q", check.name, check.got, check.want)
		}
	}

	// HDR is compared even when the manifest omits it: only HDR cases name a format.
	if video.HDR != media.HDRFormat(expected.HDR) {
		t.Errorf("video hdr = %q, want %q", video.HDR, expected.HDR)
	}

	assertVideoGeometry(t, video, expected)
	assertTrack(t, "video", video, expected.Track)
}

func assertVideoGeometry(t *testing.T, video media.Stream, expected *videoExpectation) {
	t.Helper()

	if expected.Width != 0 && (video.Width != expected.Width || video.Height != expected.Height) {
		t.Errorf("video size = %dx%d, want %dx%d", video.Width, video.Height, expected.Width, expected.Height)
	}

	if expected.PixelFormat != "" {
		if want := media.BitDepth(0, expected.PixelFormat); video.BitDepth != want {
			t.Errorf("video bit depth = %d, want %d", video.BitDepth, want)
		}
	}
}

func assertAudio(t *testing.T, description media.Description, expected []audioExpectation) {
	t.Helper()

	streams := streamsOfKind(description, media.StreamKindAudio)
	if len(streams) != len(expected) {
		t.Fatalf("got %d audio streams, want %d", len(streams), len(expected))
	}

	for index, want := range expected {
		got := streams[index]
		if got.Codec != want.Codec {
			t.Errorf("audio %d codec = %q, want %q", index, got.Codec, want.Codec)
		}

		if want.SampleRate != 0 && got.SampleRate != want.SampleRate {
			t.Errorf("audio %d sample rate = %d, want %d", index, got.SampleRate, want.SampleRate)
		}

		if want.Channels != 0 && got.Channels != want.Channels {
			t.Errorf("audio %d channels = %d, want %d", index, got.Channels, want.Channels)
		}

		if want.BitDepth != 0 && got.BitDepth != want.BitDepth {
			t.Errorf("audio %d bit depth = %d, want %d", index, got.BitDepth, want.BitDepth)
		}

		assertTrack(t, "audio", got, want.Track)
	}
}

func assertSubtitles(t *testing.T, description media.Description, expected []trackedCodec) {
	t.Helper()

	streams := streamsOfKind(description, media.StreamKindSubtitle)
	if len(streams) != len(expected) {
		t.Fatalf("got %d subtitle streams, want %d", len(streams), len(expected))
	}

	for index, want := range expected {
		if streams[index].Codec != want.Codec {
			t.Errorf("subtitle %d codec = %q, want %q", index, streams[index].Codec, want.Codec)
		}

		assertTrack(t, "subtitle", streams[index], want.Track)
	}
}

func assertTrack(t *testing.T, kind string, stream media.Stream, expected *trackExpectation) {
	t.Helper()

	if expected == nil {
		return
	}

	if stream.Language != expected.Language || stream.Default != expected.Default || stream.Forced != expected.Forced {
		t.Errorf("%s stream %d track = %q default %t forced %t, want %q default %t forced %t", kind,
			stream.Index, stream.Language, stream.Default, stream.Forced,
			expected.Language, expected.Default, expected.Forced)
	}
}

func assertSidecars(t *testing.T, prober *ffmpeg.Prober, sidecars []sidecarExpectation) {
	t.Helper()

	for _, sidecar := range sidecars {
		description, err := prober.Probe(t.Context(), corpusPath(t, sidecar.File))
		if err != nil {
			t.Errorf("Probe(%s): %v", sidecar.File, err)

			continue
		}

		if description.FormatName != sidecar.ProbeFormat || len(description.Streams) != 1 ||
			description.Streams[0].Codec != sidecar.Codec {
			t.Errorf("sidecar %s = %q %+v, want %q with codec %q", sidecar.File,
				description.FormatName, description.Streams, sidecar.ProbeFormat, sidecar.Codec)
		}
	}
}

func streamsOfKind(description media.Description, kind media.StreamKind) []media.Stream {
	var streams []media.Stream

	for _, stream := range description.Streams {
		if stream.Kind == kind {
			streams = append(streams, stream)
		}
	}

	return streams
}

// TestProbeHostileFileNames proves that a file name is never read as an option or a
// protocol: FFmpeg receives every input as file:<absolute path> (ADR 0022).
func TestProbeHostileFileNames(t *testing.T) {
	ffprobePath, manifest := requireCorpus(t)
	source := corpusPath(t, corpusCaseByName(t, manifest, "video-mkv-h264-aac").File)
	prober := ffmpeg.NewProber(ffprobePath)

	names := []string{"-dash.mkv", "-i.mkv", "Ümlaut ñ 日本.mkv", "100% #hash;x.mkv"}
	if runtime.GOOS != "windows" {
		// Windows forbids ':', '?' and '|' in file names. Elsewhere "concat:" would select
		// FFmpeg's concat protocol, '?' could start a URL query and a leading '|' marks a
		// pipe in some tools.
		names = append(names, "concat:x.mkv", "what?.mkv", "|pipe.mkv")
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := copyFile(t, source, t.TempDir(), name)

			description, err := prober.Probe(t.Context(), path)
			if err != nil {
				t.Fatalf("Probe: %v", err)
			}

			video, found := description.PrimaryVideo()
			if description.Container != media.ContainerMKV || !found || video.Codec != "h264" {
				t.Errorf("description = %+v", description)
			}

			if (description.Duration - 2*time.Second).Abs() > durationTolerance {
				t.Errorf("duration = %s", description.Duration)
			}
		})
	}
}
