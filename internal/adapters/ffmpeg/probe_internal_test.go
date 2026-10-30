package ffmpeg

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

// The probe-*.json fixtures except probe-edge-values.json were captured from the generated
// corpus with FFmpeg 9.0.2, run inside testdata/media:
//
//	ffprobe -v error -hide_banner -print_format json -show_format -show_streams file:<corpus file>
//
// probe-edge-values.json is hand-written from real ffprobe output shapes to cover values the
// corpus cannot produce: "N/A" numbers, level -99, an r_frame_rate with a zero denominator,
// "und" and "unknown" placeholders, upper-case Matroska tag keys, cover art and a font
// attachment.

func TestDescribeMapsFixtures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fixture   string
		extension string
		want      media.Description
	}{
		{fixture: "probe-mkv-hevc10-hdr10-ac3.json", extension: ".mkv", want: hevcHDR10Description()},
		{fixture: "probe-mp4-mov-text.json", extension: ".mp4", want: movTextDescription()},
		{fixture: "probe-mkv-multitrack.json", extension: ".mkv", want: multitrackDescription()},
		{fixture: "probe-avi-xvid.json", extension: ".avi", want: xvidDescription()},
		{fixture: "probe-m4b-bin-data.json", extension: ".m4b", want: audiobookDescription()},
		{fixture: "probe-ts-mpeg2.json", extension: ".ts", want: transportStreamDescription()},
		{fixture: "probe-edge-values.json", extension: ".mkv", want: edgeValuesDescription()},
		{fixture: "probe-flac-24bit.json", extension: ".flac", want: flacDescription()},
	}

	for _, test := range tests {
		t.Run(test.fixture, func(t *testing.T) {
			t.Parallel()

			got, err := describe(readFixture(t, test.fixture), test.extension)
			if err != nil {
				t.Fatalf("describe: %v", err)
			}

			assertDescription(t, got, test.want)

			if err := got.Validate(); err != nil {
				t.Errorf("description does not validate: %v", err)
			}
		})
	}
}

func TestDescribeRefinesContainerByExtension(t *testing.T) {
	t.Parallel()

	got, err := describe(readFixture(t, "probe-mkv-multitrack.json"), ".WEBM")
	if err != nil {
		t.Fatalf("describe: %v", err)
	}

	if got.Container != media.ContainerWebM {
		t.Errorf("container = %q, want webm", got.Container)
	}
}

func TestDescribeRejectsUnusableOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
	}{
		{name: "malformed JSON", output: `{"streams": [`},
		{name: "not an object", output: `[]`},
		{name: "empty", output: ``},
		{name: "missing format", output: `{"streams": []}`},
		{name: "wrong type", output: `{"streams": [{"index": "zero"}], "format": {}}`},
		{name: "duplicate index", output: `{"streams": [{"index": 0}, {"index": 0}], "format": {"format_name": "avi"}}`},
		{name: "negative index", output: `{"streams": [{"index": -1}], "format": {"format_name": "avi"}}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := describe([]byte(test.output), ".mkv")
			if !errors.Is(err, domain.ErrUnsupported) {
				t.Errorf("error = %v, want ErrUnsupported", err)
			}
		})
	}
}

func TestDescribeAcceptsFileWithoutStreams(t *testing.T) {
	t.Parallel()

	got, err := describe([]byte(`{"format": {"format_name": "mpegts", "duration": "0.000000"}}`), ".ts")
	if err != nil {
		t.Fatalf("describe: %v", err)
	}

	if got.Container != media.ContainerMPEGTS || len(got.Streams) != 0 || got.Duration != 0 {
		t.Errorf("description = %+v", got)
	}
}

func TestFrameRateFallsBackAndNeverFails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		average string
		real    string
		want    media.FrameRate
	}{
		{name: "average wins", average: "24000/1001", real: "24/1", want: media.NewFrameRate(24000, 1001)},
		{name: "unknown average", average: "0/0", real: "25/1", want: media.NewFrameRate(25, 1)},
		{name: "malformed average", average: "abc", real: "30/1", want: media.NewFrameRate(30, 1)},
		{name: "zero denominator", average: "0/0", real: "25/0", want: media.FrameRate{}},
		{name: "both malformed", average: "25/0", real: "-1/1", want: media.FrameRate{}},
		{name: "missing", want: media.FrameRate{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := frameRate(test.average, test.real); got != test.want {
				t.Errorf("frameRate(%q, %q) = %v, want %v", test.average, test.real, got, test.want)
			}
		})
	}
}

func TestParseSeconds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		text   string
		want   time.Duration
		wantOK bool
	}{
		{text: "2.005000", want: 2005 * time.Millisecond, wantOK: true},
		{text: "1.429978", want: 1429978 * time.Microsecond, wantOK: true},
		{text: "30", want: 30 * time.Second, wantOK: true},
		{text: "-0.080000", want: -80 * time.Millisecond, wantOK: true},
		{text: " 1.5 ", want: 1500 * time.Millisecond, wantOK: true},
		{text: "N/A"},
		{text: ""},
		{text: "1h"},
		{text: "1e3"},
		{text: "1.5.0"},
		{text: "+"},
	}

	for _, test := range tests {
		t.Run(test.text, func(t *testing.T) {
			t.Parallel()

			got, ok := parseSeconds(test.text)
			if got != test.want || ok != test.wantOK {
				t.Errorf("parseSeconds(%q) = %s, %t, want %s, %t", test.text, got, ok, test.want, test.wantOK)
			}
		})
	}
}

func TestCodecTag(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"XVID":          "XVID",
		"avc1":          "avc1",
		"[0][0][0][0]":  "",
		"[2][0][0][0]":  "",
		"U[0][0][0]":    "",
		"":              "",
		" DX50 ":        "DX50",
		"[27][0][0][0]": "",
	}

	for input, want := range tests {
		if got := codecTag(input); got != want {
			t.Errorf("codecTag(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestTagLookupIsCaseInsensitiveAndDeterministic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tags map[string]string
		want string
	}{
		{name: "exact key wins", tags: map[string]string{"language": "eng", "LANGUAGE": "deu", "Language": "fra"}, want: "eng"},
		// Without the exact key the smallest spelling wins: "LANGUAGE" < "Language".
		{name: "smallest spelling wins", tags: map[string]string{"LANGUAGE": "deu", "Language": "fra"}, want: "deu"},
		{name: "single other spelling", tags: map[string]string{"Language": "fra"}, want: "fra"},
		{name: "missing", tags: map[string]string{"title": "x"}, want: ""},
		{name: "nil tags", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			for range 20 { // map order varies between iterations
				if got := tag(test.tags, "language"); got != test.want {
					t.Fatalf("tag(%v, language) = %q, want %q", test.tags, got, test.want)
				}
			}
		})
	}
}

func TestProbeRunsFFprobeWithSafeArguments(t *testing.T) {
	fake, calls := fakeResponder(t, map[string]string{
		fakeDefaultResponse: string(readFixture(t, "probe-mkv-multitrack.json")),
	})
	path := filepath.Join(t.TempDir(), "-dash clip.webm")

	got, err := NewProber(fake.executable).Probe(t.Context(), path)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}

	want := []string{
		"-v", "error", "-hide_banner", "-print_format", "json", "-show_format", "-show_streams",
		"-protocol_whitelist", "file", "file:" + path,
	}
	if recorded := calls(); len(recorded) != 1 || !slices.Equal(recorded[0], want) {
		t.Errorf("arguments = %q, want %q", recorded, want)
	}

	if got.Container != media.ContainerWebM || len(got.Streams) != 5 {
		t.Errorf("description = %+v", got)
	}
}

func TestProbeRejectsRelativePath(t *testing.T) {
	t.Parallel()

	_, err := NewProber("ffprobe-that-is-never-started").Probe(t.Context(), filepath.Join("video", "clip.mkv"))
	if !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("error = %v, want ErrInvalid", err)
	}
}

func TestProbeRejectsWindowsDevicePaths(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "windows" {
		t.Skip("device namespace paths exist on Windows only")
	}

	for _, path := range []string{`\\.\COM1`, `\\.\PhysicalDrive0`, `//./nul`, `\\?\GLOBALROOT\Device\Null`, `\\?\globalroot\Device\Null`} {
		if _, err := NewProber("ffprobe-that-is-never-started").Probe(t.Context(), path); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("Probe(%q) error = %v, want ErrInvalid", path, err)
		}
	}

	// Long paths and UNC shares are ordinary files.
	for _, path := range []string{`\\?\C:\media\clip.mkv`, `\\nas\media\clip.mkv`} {
		if _, err := inputArguments(path); err != nil {
			t.Errorf("inputArguments(%q) = %v, want no error", path, err)
		}
	}
}

func TestProbeErrorsNameOnlyTheBaseName(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "secret library root")
	path := filepath.Join(directory, "clip.mkv")
	prober := NewProber(fakeTool(t, fakeModeFail).executable)

	_, probeErr := prober.Probe(t.Context(), path)
	_, keyframeErr := prober.KeyframeAtOrBefore(t.Context(), path, 21*time.Second)

	for _, err := range []error{probeErr, keyframeErr} {
		if err == nil {
			t.Fatal("expected an error")
		}

		if message := err.Error(); strings.Contains(message, directory) || !strings.Contains(message, "clip.mkv") {
			t.Errorf("error %q must name clip.mkv but not %q", message, directory)
		}

		// The debug output is reachable on purpose and may contain the full path.
		if output, ok := ErrorOutput(err); !ok || output == "" {
			t.Errorf("ErrorOutput(%v) = %q, %t, want the stderr tail", err, output, ok)
		}
	}
}

func TestProbeClassifiesFailures(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T) *Prober
		want  error
	}{
		{
			name: "ffprobe fails",
			setup: func(t *testing.T) *Prober {
				return NewProber(fakeTool(t, fakeModeFail).executable)
			},
			want: domain.ErrUnsupported,
		},
		{
			name: "malformed output",
			setup: func(t *testing.T) *Prober {
				fake, _ := fakeResponder(t, map[string]string{fakeDefaultResponse: "{"})

				return NewProber(fake.executable)
			},
			want: domain.ErrUnsupported,
		},
		{
			name: "timeout",
			setup: func(t *testing.T) *Prober {
				prober := NewProber(fakeTool(t, fakeModeHang).executable)
				prober.probeTimeout = 200 * time.Millisecond

				return prober
			},
			want: context.DeadlineExceeded,
		},
		{
			name: "missing executable",
			setup: func(t *testing.T) *Prober {
				return NewProber(filepath.Join(t.TempDir(), "no-such-ffprobe"))
			},
			want: domain.ErrUnavailable,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "clip.mkv")

			_, err := test.setup(t).Probe(t.Context(), path)
			if !errors.Is(err, test.want) {
				t.Errorf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func assertDescription(t *testing.T, got, want media.Description) {
	t.Helper()

	if got.Container != want.Container || got.FormatName != want.FormatName ||
		got.Duration != want.Duration || got.BitRate != want.BitRate {
		t.Errorf("format = %q %q %s %d, want %q %q %s %d", got.Container, got.FormatName, got.Duration,
			got.BitRate, want.Container, want.FormatName, want.Duration, want.BitRate)
	}

	if len(got.Streams) != len(want.Streams) {
		t.Fatalf("got %d streams, want %d: %+v", len(got.Streams), len(want.Streams), got.Streams)
	}

	for index := range want.Streams {
		if !reflect.DeepEqual(got.Streams[index], want.Streams[index]) {
			t.Errorf("stream %d:\n got %+v\nwant %+v", index, got.Streams[index], want.Streams[index])
		}
	}
}

func h264Stream(index int) media.Stream {
	return media.Stream{
		Index: index, Kind: media.StreamKindVideo, Codec: "h264", Profile: "High", Level: 13,
		PixelFormat: "yuv420p", BitDepth: 8, Width: 320, Height: 240,
		FrameRate: media.NewFrameRate(25, 1),
	}
}

func stereoStream(index int, codec string) media.Stream {
	return media.Stream{
		Index: index, Kind: media.StreamKindAudio, Codec: codec,
		Channels: 2, ChannelLayout: "stereo", SampleRate: 48000,
	}
}

func hevcHDR10Description() media.Description {
	audio := stereoStream(1, "ac3")
	audio.BitRate = 192000

	return media.Description{
		Container: media.ContainerMKV, FormatName: "matroska,webm",
		Duration: 2005 * time.Millisecond, BitRate: 453398,
		Streams: []media.Stream{
			{
				Index: 0, Kind: media.StreamKindVideo, Codec: "hevc", Profile: "Main 10", Level: 60,
				PixelFormat: "yuv420p10le", BitDepth: 10, Width: 320, Height: 240,
				FrameRate: media.NewFrameRate(25, 1), ColorPrimaries: "bt2020",
				ColorTransfer: "smpte2084", ColorSpace: "bt2020nc", HDR: media.HDRFormatHDR10,
			},
			audio,
		},
	}
}

func movTextDescription() media.Description {
	video := h264Stream(0)
	video.CodecTag, video.BitRate, video.Default = "avc1", 262308, true

	audio := stereoStream(1, "aac")
	audio.CodecTag, audio.Profile, audio.BitRate, audio.Default = "mp4a", "LC", 128469, true

	return media.Description{
		Container: media.ContainerMP4, FormatName: "mov,mp4,m4a,3gp,3g2,mj2",
		Duration: 2 * time.Second, BitRate: 406248,
		Streams: []media.Stream{
			video,
			audio,
			{
				Index: 2, Kind: media.StreamKindSubtitle, Codec: "mov_text", CodecTag: "tx3g",
				Language: "eng", Default: true, BitRate: 218,
			},
		},
	}
}

func multitrackDescription() media.Description {
	video := h264Stream(0)
	video.Default = true

	english := stereoStream(1, "aac")
	english.Profile, english.Language, english.Default = "LC", "eng", true

	german := stereoStream(2, "aac")
	german.Profile, german.Language = "LC", "deu"

	return media.Description{
		Container: media.ContainerMKV, FormatName: "matroska,webm",
		Duration: 2021 * time.Millisecond, BitRate: 528601,
		Streams: []media.Stream{
			video, english, german,
			{Index: 3, Kind: media.StreamKindSubtitle, Codec: "subrip", Language: "eng"},
			{Index: 4, Kind: media.StreamKindSubtitle, Codec: "subrip", Language: "deu", Forced: true},
		},
	}
}

func xvidDescription() media.Description {
	audio := stereoStream(1, "mp3")
	audio.BitRate = 129524

	return media.Description{
		Container: media.ContainerAVI, FormatName: "avi",
		Duration: 2040 * time.Millisecond, BitRate: 654337,
		Streams: []media.Stream{
			{
				Index: 0, Kind: media.StreamKindVideo, Codec: "mpeg4", CodecTag: "XVID",
				Profile: "Simple Profile", Level: 1, PixelFormat: "yuv420p", BitDepth: 8,
				Width: 320, Height: 240, FrameRate: media.NewFrameRate(25, 1), BitRate: 483820,
			},
			audio,
		},
	}
}

func audiobookDescription() media.Description {
	audio := stereoStream(0, "aac")
	audio.CodecTag, audio.Profile, audio.BitRate, audio.Default = "mp4a", "LC", 128040, true

	return media.Description{
		Container: media.ContainerMP4, FormatName: "mov,mp4,m4a,3gp,3g2,mj2",
		Duration: 5 * time.Second, BitRate: 132558,
		Streams: []media.Stream{
			audio,
			{
				Index: 1, Kind: media.StreamKindOther, Codec: "bin_data", CodecTag: "text",
				Language: "eng", BitRate: 73,
			},
		},
	}
}

func transportStreamDescription() media.Description {
	audio := stereoStream(1, "mp2")
	audio.BitRate = 384000

	return media.Description{
		Container: media.ContainerMPEGTS, FormatName: "mpegts",
		Duration: 2010022 * time.Microsecond, BitRate: 957760,
		Streams: []media.Stream{
			{
				Index: 0, Kind: media.StreamKindVideo, Codec: "mpeg2video", Profile: "Main", Level: 8,
				PixelFormat: "yuv420p", BitDepth: 8, Width: 320, Height: 240,
				FrameRate: media.NewFrameRate(25, 1),
			},
			audio,
		},
	}
}

// flacDescription: bits_per_raw_sample is the sample depth of lossless audio.
func flacDescription() media.Description {
	audio := stereoStream(0, "flac")
	audio.SampleRate, audio.BitDepth = 96000, 24

	return media.Description{
		Container: media.ContainerUnknown, FormatName: "flac",
		Duration: 5 * time.Second, BitRate: 957881,
		Streams: []media.Stream{audio},
	}
}

func edgeValuesDescription() media.Description {
	return media.Description{
		Container: media.ContainerMKV, FormatName: "matroska,webm",
		Streams: []media.Stream{
			{
				Index: 0, Kind: media.StreamKindVideo, Codec: "h264", PixelFormat: "yuv420p10le",
				BitDepth: 10, Width: 1920, Height: 1080, ColorPrimaries: "bt2020",
				ColorTransfer: "arib-std-b67", HDR: media.HDRFormatHLG, Title: "Main feature",
				Default: true,
			},
			{Index: 1, Kind: media.StreamKindAudio, Codec: "mp2"},
			{
				Index: 2, Kind: media.StreamKindVideo, Codec: "mjpeg", Profile: "Baseline",
				PixelFormat: "yuvj420p", BitDepth: 8, Width: 600, Height: 600,
				FrameRate: media.NewFrameRate(90000, 1), AttachedPicture: true,
			},
			{Index: 3, Kind: media.StreamKindOther},
		},
	}
}
