package ffmpeg

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// The ffmpeg-<version>-{version,encoders,muxers}.txt fixtures are real FFmpeg output. Their
// first line, starting with "#", names the command that captured them and is removed before
// parsing (withoutFixtureHeader); everything after it is verbatim.

// fixtureHeaderPrefix starts the provenance lines at the top of a capability fixture.
const fixtureHeaderPrefix = "#"

func TestCapabilityOutputsParseFixtures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		version string
		want    playback.ServerCapabilities
	}{
		{
			version: "9.0.2",
			want: playback.ServerCapabilities{
				FFmpegVersion: "9.0.2-full_build-www.gyan.dev",
				HLSMuxing:     true,
				H264Encoder:   playback.EncoderLibx264,
				AACEncoder:    playback.EncoderAAC,
			},
		},
		{
			version: "7.1.5",
			want: playback.ServerCapabilities{
				FFmpegVersion: "7.1.5-0+deb13u1",
				HLSMuxing:     true,
				H264Encoder:   playback.EncoderLibx264,
				AACEncoder:    playback.EncoderAAC,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.version, func(t *testing.T) {
			t.Parallel()

			outputs := capabilityOutputs{
				version:  capabilityFixture(t, test.version, "version"),
				encoders: capabilityFixture(t, test.version, "encoders"),
				muxers:   capabilityFixture(t, test.version, "muxers"),
			}

			got, err := outputs.parse()
			if err != nil {
				t.Fatalf("parse: %v", err)
			}

			if got != test.want {
				t.Errorf("capabilities = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestParseListingReadsFixtures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fixture string
		// present must be listed with the given flags; absent must not be listed at all.
		present map[string]string
		absent  []string
		count   int
	}{
		{
			fixture: "ffmpeg-9.0.2-encoders.txt",
			present: map[string]string{"libx264": "V....D", "aac": "A....D", "libsvtav1": "V....."},
			absent:  []string{"h264", "Multicolor", "------"},
			count:   241,
		},
		{
			fixture: "ffmpeg-7.1.5-encoders.txt",
			present: map[string]string{"libx264": "V....D", "aac": "A....D", "amv": "V.....", "opus": "A..X.D"},
			absent:  []string{"h264"},
			count:   225,
		},
		{
			fixture: "ffmpeg-9.0.2-muxers.txt",
			present: map[string]string{"hls": " E ", "mp4": " E ", "ssegment": " E ", "segment": " E "},
			absent:  []string{"stream_segment,ssegment", "D..", "---"},
			count:   185,
		},
		{
			fixture: "ffmpeg-7.1.5-muxers.txt",
			present: map[string]string{"hls": " E ", "mp4": " E ", "alsa": " Ed", "v4l2": " Ed"},
			absent:  []string{"QuickTime"},
			count:   189,
		},
	}

	for _, test := range tests {
		t.Run(test.fixture, func(t *testing.T) {
			t.Parallel()

			entries, err := parseListing(withoutFixtureHeader(readFixture(t, test.fixture)))
			if err != nil {
				t.Fatalf("parseListing: %v", err)
			}

			if len(entries) != test.count {
				t.Errorf("entries = %d, want %d", len(entries), test.count)
			}

			assertListing(t, entries, test.present, test.absent)
		})
	}
}

// assertListing checks that every present name is listed with its flags and that no absent
// name is listed at all.
func assertListing(t *testing.T, entries []listingEntry, present map[string]string, absent []string) {
	t.Helper()

	for name, flags := range present {
		if got, found := listedFlags(entries, name); !found || got != flags {
			t.Errorf("%s: flags = %q (listed %t), want %q", name, got, found, flags)
		}
	}

	for _, name := range absent {
		if _, found := listedFlags(entries, name); found {
			t.Errorf("%s is listed, want absent", name)
		}
	}
}

func TestParseVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		output  string
		want    string
		wantErr error
	}{
		{
			name:   "release",
			output: "ffmpeg version 7.1.5-0+deb13u1 Copyright (c) 2000-2026 the FFmpeg developers\nbuilt with gcc 14\n",
			want:   "7.1.5-0+deb13u1",
		},
		{
			name:   "git build",
			output: "ffmpeg version N-121443-g0123456789-20261001 Copyright (c) 2000-2026\n",
			want:   "N-121443-g0123456789-20261001",
		},
		{
			name:   "CRLF line endings",
			output: "ffmpeg version 9.0.2 Copyright\r\nbuilt with gcc\r\n",
			want:   "9.0.2",
		},
		{
			name:   "no trailing newline",
			output: "ffmpeg version 6.1",
			want:   "6.1",
		},
		{
			name:   "warning before the banner",
			output: "Warning: something odd\nffmpeg version 8.0 Copyright\n",
			want:   "8.0",
		},
		{
			name:   "over-long version is cut",
			output: "ffmpeg version " + strings.Repeat("9", maxVersionLength+10) + " Copyright\n",
			want:   strings.Repeat("9", maxVersionLength),
		},
		{name: "empty output", output: "", wantErr: errNoVersionBanner},
		{name: "other program", output: "ffprobe version 9.0.2 Copyright\n", wantErr: errNoVersionBanner},
		{name: "banner not at line start", output: "x ffmpeg version 9.0.2\n", wantErr: errNoVersionBanner},
		{name: "banner without version", output: "ffmpeg version \n", wantErr: errNoVersionBanner},
		{name: "control character", output: "ffmpeg version 9.0\x1b[2J Copyright\n", wantErr: errMalformedVersion},
		{name: "non-ASCII version", output: "ffmpeg version 9.0é Copyright\n", wantErr: errMalformedVersion},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseVersion([]byte(test.output))
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}

			if got != test.want {
				t.Errorf("version = %q, want %q", got, test.want)
			}
		})
	}
}

// encoderListing builds -encoders output with the real legend and separator.
func encoderListing(lines ...string) string {
	return "Encoders:\n V..... = Video\n A..... = Audio\n ...X.. = Codec is experimental\n ------\n" +
		strings.Join(lines, "\n") + "\n"
}

// muxerListing builds -muxers output with the real legend and separator.
func muxerListing(lines ...string) string {
	return "Formats:\n D.. = Demuxing supported\n .E. = Muxing supported\n ..d = Is a device\n ---\n" +
		strings.Join(lines, "\n") + "\n"
}

func TestParseCapabilitiesMatchesTheNameColumn(t *testing.T) {
	t.Parallel()

	const (
		allMuxers = "  E  hls             Apple HTTP Live Streaming\n  E  mp4             MP4 (MPEG-4 Part 14)"
		x264      = " V....D libx264              libx264 H.264 (codec h264)"
		aac       = " A....D aac                  AAC (Advanced Audio Coding)"
	)

	tests := []struct {
		name     string
		encoders string
		muxers   string
		want     playback.ServerCapabilities
	}{
		{
			name:     "everything present",
			encoders: encoderListing(x264, aac),
			muxers:   muxerListing(allMuxers),
			want:     capabilitiesWith(true, playback.EncoderLibx264, playback.EncoderAAC),
		},
		{
			name: "names only in other columns or as prefixes",
			encoders: encoderListing(
				" V....D libx264rgb           libx264 H.264 RGB (codec h264)",
				" A....D libfdk_aac           Fraunhofer FDK AAC (codec aac)",
				" V....D h264_nvenc           NVIDIA NVENC libx264 aac (codec h264)",
			),
			muxers: muxerListing(
				"  E  hls_old         mp4 hls",
				"  E  ismv            ISMV/ISMA (Smooth Streaming) mp4",
			),
			want: capabilitiesWith(false, "", ""),
		},
		{
			name:     "encoders of the wrong kind",
			encoders: encoderListing(" A....D libx264              fake", " V....D aac                  fake"),
			muxers:   muxerListing(allMuxers),
			want:     capabilitiesWith(true, "", ""),
		},
		{
			name:     "experimental encoders are not offered",
			encoders: encoderListing(" V..X.D libx264              libx264", " A..X.. aac                  AAC"),
			muxers:   muxerListing(allMuxers),
			want:     capabilitiesWith(true, "", ""),
		},
		{
			name:     "hls without mp4",
			encoders: encoderListing(x264, aac),
			muxers:   muxerListing("  E  hls             Apple HTTP Live Streaming"),
			want:     capabilitiesWith(false, playback.EncoderLibx264, playback.EncoderAAC),
		},
		{
			name:     "listed without the muxing flag",
			encoders: encoderListing(x264, aac),
			muxers:   muxerListing(" D   hls             Apple HTTP Live Streaming", " D   mp4             MP4"),
			want:     capabilitiesWith(false, playback.EncoderLibx264, playback.EncoderAAC),
		},
		{
			name:     "muxer aliases separated by commas",
			encoders: encoderListing(x264, aac),
			muxers:   muxerListing("  E  mov,mp4         combined", "  E  apple,hls       combined"),
			want:     capabilitiesWith(true, playback.EncoderLibx264, playback.EncoderAAC),
		},
		{
			name:     "CRLF line endings",
			encoders: strings.ReplaceAll(encoderListing(x264, aac), "\n", "\r\n"),
			muxers:   strings.ReplaceAll(muxerListing(allMuxers), "\n", "\r\n"),
			want:     capabilitiesWith(true, playback.EncoderLibx264, playback.EncoderAAC),
		},
		{
			name:     "minimal NAS build without encoders",
			encoders: encoderListing(" V....D mpeg4                MPEG-4 part 2"),
			muxers:   muxerListing(allMuxers),
			want:     capabilitiesWith(true, "", ""),
		},
		{
			name:     "lines before the separator are legend, not entries",
			encoders: "Encoders:\n V....D libx264 legend\n A....D aac legend\n ------\n",
			muxers:   "Formats:\n  E  hls legend\n  E  mp4 legend\n ---\n",
			want:     capabilitiesWith(false, "", ""),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			outputs := capabilityOutputs{
				version:  []byte("ffmpeg version 9.0.2 Copyright\n"),
				encoders: []byte(test.encoders),
				muxers:   []byte(test.muxers),
			}

			got, err := outputs.parse()
			if err != nil {
				t.Fatalf("parse: %v", err)
			}

			if got != test.want {
				t.Errorf("capabilities = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestParseListingSkipsMalformedLines(t *testing.T) {
	t.Parallel()

	output := encoderListing(
		"",
		" V....D",
		" V....D ",
		"V....D  shifted              no leading space",
		" V....Dx glued               no space after the flags",
		" V....D  indented            name column is a space",
		" V..1.D digits               digits are no flags",
		"Exiting with exit code 0",
		" A....D aac                  AAC",
	)

	entries, err := parseListing([]byte(output))
	if err != nil {
		t.Fatalf("parseListing: %v", err)
	}

	want := []listingEntry{{flags: "A....D", names: []string{"aac"}}}
	if !slices.EqualFunc(entries, want, func(a, b listingEntry) bool {
		return a.flags == b.flags && slices.Equal(a.names, b.names)
	}) {
		t.Errorf("entries = %+v, want %+v", entries, want)
	}
}

func TestCapabilityOutputsParseRejectsUnknownOutput(t *testing.T) {
	t.Parallel()

	valid := capabilityOutputs{
		version:  []byte("ffmpeg version 9.0.2\n"),
		encoders: []byte(encoderListing()),
		muxers:   []byte(muxerListing()),
	}

	tests := []struct {
		name    string
		modify  func(*capabilityOutputs)
		wantErr error
	}{
		{name: "no version banner", modify: func(o *capabilityOutputs) { o.version = []byte("usage: ffmpeg") }, wantErr: errNoVersionBanner},
		{name: "encoders without separator", modify: func(o *capabilityOutputs) { o.encoders = []byte("Encoders:\n V....D libx264 x\n") }, wantErr: errNoListing},
		{name: "muxers without separator", modify: func(o *capabilityOutputs) { o.muxers = []byte("") }, wantErr: errNoListing},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			outputs := valid
			test.modify(&outputs)

			got, err := outputs.parse()
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}

			if got != (playback.ServerCapabilities{}) {
				t.Errorf("capabilities = %+v, want zero", got)
			}
		})
	}
}

// scriptedFFmpeg answers capability queries by their last argument and records the calls.
type scriptedFFmpeg struct {
	outputs map[string]string
	// failures makes the query with this last argument return the error.
	failures map[string]error
	calls    [][]string
	// deadlines records whether each call ran under a deadline.
	deadlines []bool
}

func (s *scriptedFFmpeg) run(ctx context.Context, arguments []string) ([]byte, error) {
	s.calls = append(s.calls, slices.Clone(arguments))
	_, hasDeadline := ctx.Deadline()
	s.deadlines = append(s.deadlines, hasDeadline)

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	query := arguments[len(arguments)-1]
	if err := s.failures[query]; err != nil {
		return nil, err
	}

	return []byte(s.outputs[query]), nil
}

func healthyFFmpeg() *scriptedFFmpeg {
	return &scriptedFFmpeg{outputs: map[string]string{
		"-version":  "ffmpeg version 9.0.2 Copyright\n",
		"-encoders": encoderListing(" V....D libx264              x", " A....D aac                  y"),
		"-muxers":   muxerListing("  E  hls             a", "  E  mp4             b"),
	}}
}

func TestDetectCapabilitiesRunsTheThreeQueries(t *testing.T) {
	t.Parallel()

	ffmpeg := healthyFFmpeg()

	got, err := detectCapabilities(t.Context(), ffmpeg.run, time.Minute)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}

	want := playback.ServerCapabilities{
		FFmpegVersion: "9.0.2", HLSMuxing: true,
		H264Encoder: playback.EncoderLibx264, AACEncoder: playback.EncoderAAC,
	}
	if got != want {
		t.Errorf("capabilities = %+v, want %+v", got, want)
	}

	wantCalls := [][]string{{"-hide_banner", "-version"}, {"-hide_banner", "-encoders"}, {"-hide_banner", "-muxers"}}
	if !slices.EqualFunc(ffmpeg.calls, wantCalls, slices.Equal) {
		t.Errorf("calls = %q, want %q", ffmpeg.calls, wantCalls)
	}

	if slices.Contains(ffmpeg.deadlines, false) {
		t.Errorf("deadlines = %v, want every query bounded by a timeout", ffmpeg.deadlines)
	}
}

func TestDetectCapabilitiesFailsClosed(t *testing.T) {
	t.Parallel()

	failure := errors.New("ffmpeg exited with code 1")
	startFailure := errors.Join(domain.ErrUnavailable, errors.New("executable file not found"))

	tests := []struct {
		name      string
		configure func(*scriptedFFmpeg)
		wantErr   error
		wantCalls int
	}{
		{
			name:      "cannot start",
			configure: func(s *scriptedFFmpeg) { s.failures = map[string]error{"-version": startFailure} },
			wantErr:   startFailure,
			wantCalls: 1,
		},
		{
			name:      "encoders query fails",
			configure: func(s *scriptedFFmpeg) { s.failures = map[string]error{"-encoders": failure} },
			wantErr:   failure,
			wantCalls: 2,
		},
		{
			name:      "muxers query hits the timeout",
			configure: func(s *scriptedFFmpeg) { s.failures = map[string]error{"-muxers": context.DeadlineExceeded} },
			wantErr:   context.DeadlineExceeded,
			wantCalls: 3,
		},
		{
			name:      "output not understood",
			configure: func(s *scriptedFFmpeg) { s.outputs["-version"] = "Usage: something else\n" },
			wantErr:   errNoVersionBanner,
			wantCalls: 3,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ffmpeg := healthyFFmpeg()
			test.configure(ffmpeg)

			got, err := detectCapabilities(t.Context(), ffmpeg.run, time.Minute)
			if !errors.Is(err, domain.ErrUnavailable) || !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want ErrUnavailable wrapping %v", err, test.wantErr)
			}

			if got != (playback.ServerCapabilities{}) {
				t.Errorf("capabilities = %+v, want zero", got)
			}

			if len(ffmpeg.calls) != test.wantCalls {
				t.Errorf("calls = %d, want %d", len(ffmpeg.calls), test.wantCalls)
			}
		})
	}
}

func TestDetectCapabilitiesReportsCallerCancellationAsIs(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	got, err := detectCapabilities(ctx, healthyFFmpeg().run, time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}

	// Shutting down during startup says nothing about FFmpeg.
	if errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("error = %v, must not claim FFmpeg is unavailable", err)
	}

	if got != (playback.ServerCapabilities{}) {
		t.Errorf("capabilities = %+v, want zero", got)
	}
}

func TestDetectCapabilitiesWithMissingExecutable(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "no-such-ffmpeg")

	for _, path := range []string{missing, ""} {
		got, err := DetectCapabilities(t.Context(), path)
		if !errors.Is(err, domain.ErrUnavailable) {
			t.Errorf("DetectCapabilities(%q) error = %v, want ErrUnavailable", path, err)
		}

		if got != (playback.ServerCapabilities{}) {
			t.Errorf("DetectCapabilities(%q) = %+v, want zero", path, got)
		}
	}
}

func TestDetectCapabilitiesThroughAProcess(t *testing.T) {
	t.Run("failing tool keeps its error output", func(t *testing.T) {
		executable := fakeTool(t, fakeModeFail).executable

		_, err := DetectCapabilities(t.Context(), executable)
		if !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("error = %v, want ErrUnavailable", err)
		}

		if tail, ok := ErrorOutput(err); !ok || !strings.Contains(tail, "Invalid data") {
			t.Errorf("ErrorOutput = %q, %t; want the tool's error output", tail, ok)
		}
	})

	t.Run("arguments reach the process verbatim", func(t *testing.T) {
		// The fake prints the same text for every query, so the encoders listing is not
		// understood; detection stops with an error after recording the calls.
		fake, calls := fakeResponder(t, map[string]string{fakeDefaultResponse: "ffmpeg version 9.0.2\n"})

		_, err := DetectCapabilities(t.Context(), fake.executable)
		if !errors.Is(err, errNoListing) || !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("error = %v, want ErrUnavailable wrapping errNoListing", err)
		}

		want := [][]string{{"-hide_banner", "-version"}, {"-hide_banner", "-encoders"}, {"-hide_banner", "-muxers"}}
		if got := calls(); !slices.EqualFunc(got, want, slices.Equal) {
			t.Errorf("calls = %q, want %q", got, want)
		}
	})

	t.Run("hanging tool is stopped by the timeout", func(t *testing.T) {
		const timeout = 200 * time.Millisecond

		hanging := fakeTool(t, fakeModeHang)
		started := time.Now()

		_, err := detectCapabilities(t.Context(), hanging.output, timeout)
		if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("error = %v, want ErrUnavailable wrapping DeadlineExceeded", err)
		}

		if elapsed := time.Since(started); elapsed > killDeadline {
			t.Errorf("detection took %s, want the hanging tool killed", elapsed)
		}
	})
}

// capabilitiesWith builds the expected capabilities of a parsed "9.0.2" banner.
func capabilitiesWith(hlsMuxing bool, h264Encoder, aacEncoder string) playback.ServerCapabilities {
	return playback.ServerCapabilities{
		FFmpegVersion: "9.0.2",
		HLSMuxing:     hlsMuxing,
		H264Encoder:   h264Encoder,
		AACEncoder:    aacEncoder,
	}
}

// listedFlags returns the flags of the entry that lists name.
func listedFlags(entries []listingEntry, name string) (string, bool) {
	for _, entry := range entries {
		if slices.Contains(entry.names, name) {
			return entry.flags, true
		}
	}

	return "", false
}

// capabilityFixture returns the output of `ffmpeg -hide_banner -<query>` captured from
// FFmpeg version, without its provenance header.
func capabilityFixture(t *testing.T, version, query string) []byte {
	t.Helper()

	return withoutFixtureHeader(readFixture(t, "ffmpeg-"+version+"-"+query+".txt"))
}

// withoutFixtureHeader drops the leading "#" lines that record how a fixture was captured.
func withoutFixtureHeader(data []byte) []byte {
	text := string(data)

	for strings.HasPrefix(text, fixtureHeaderPrefix) {
		_, rest, found := strings.Cut(text, "\n")
		if !found {
			return nil
		}

		text = rest
	}

	return []byte(text)
}

// Guard against a fixture that silently lost the line naming its capture command.
func TestCapabilityFixturesHaveProvenance(t *testing.T) {
	t.Parallel()

	for _, version := range []string{"9.0.2", "7.1.5"} {
		for _, query := range []string{"version", "encoders", "muxers"} {
			name := "ffmpeg-" + version + "-" + query + ".txt"

			header, _, _ := strings.Cut(string(readFixture(t, name)), "\n")
			if !strings.HasPrefix(header, fixtureHeaderPrefix) || !strings.Contains(header, "-hide_banner -"+query) {
				t.Errorf("%s: header %q does not name the capture command", name, header)
			}
		}
	}
}
