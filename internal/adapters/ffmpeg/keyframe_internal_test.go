package ffmpeg

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

// probe-keyframes-ts.json and probe-keyframes-past-end.json were captured with FFmpeg 9.0.2
// inside testdata/media from the long seek clips:
//
//	ffprobe -v error -hide_banner -select_streams V:0 -skip_frame nokey
//	  -show_entries frame=key_frame,pts_time,best_effort_timestamp_time:format=start_time
//	  -read_intervals <+11.000000%+11.000000 | +40.000000%+11.000000> -print_format json
//	  file:video/<mpeg2_mp2_long.ts | h264_aac_long.mkv>
//
// The TS clip starts at 1.429978 s and has keyframes at 1.44 + 2k s. ffprobe's forward seek
// lands on 13.44 s, past the keyframe at 11.44 s – the reason the search reads a window
// before the position. probe-keyframes-none.json and probe-keyframes-edge-values.json are
// hand-written.

// tsStart is the format start time of the TS fixture.
const tsStart = 1429978 * time.Microsecond

func TestKeyframeAtOrBeforeInOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fixture   string
		output    string
		position  time.Duration
		want      time.Duration
		wantFound bool
	}{
		{name: "between keyframes", fixture: "probe-keyframes-ts.json", position: 20 * time.Second, want: 19440*time.Millisecond - tsStart, wantFound: true},
		{name: "exactly on keyframe", fixture: "probe-keyframes-ts.json", position: 21440*time.Millisecond - tsStart, want: 21440*time.Millisecond - tsStart, wantFound: true},
		{name: "after last keyframe read", fixture: "probe-keyframes-ts.json", position: 25 * time.Second, want: 21440*time.Millisecond - tsStart, wantFound: true},
		{name: "before first keyframe read", fixture: "probe-keyframes-ts.json", position: 11 * time.Second},
		{name: "no frames", fixture: "probe-keyframes-none.json", position: 20 * time.Second},
		{name: "past the end", fixture: "probe-keyframes-past-end.json", position: 40 * time.Second, want: 28 * time.Second, wantFound: true},
		{name: "missing key_frame counts as key", fixture: "probe-keyframes-edge-values.json", position: 10 * time.Second, want: 5 * time.Second, wantFound: true},
		{name: "best effort timestamp", fixture: "probe-keyframes-edge-values.json", position: 4500 * time.Millisecond, want: 4 * time.Second, wantFound: true},
		{name: "non-key frame ignored", fixture: "probe-keyframes-edge-values.json", position: 3500 * time.Millisecond},
		{
			name:      "keyframe before start time clamps to zero",
			output:    `{"frames": [{"key_frame": 1, "pts_time": "0.960000"}], "format": {"start_time": "1.000000"}}`,
			position:  500 * time.Millisecond,
			wantFound: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			output := []byte(test.output)
			if test.fixture != "" {
				output = readFixture(t, test.fixture)
			}

			got, found, err := keyframeInOutput(output, test.position)
			if err != nil {
				t.Fatalf("keyframeInOutput: %v", err)
			}

			if got != test.want || found != test.wantFound {
				t.Errorf("keyframeInOutput(%s) = %s, %t, want %s, %t", test.position, got, found, test.want, test.wantFound)
			}
		})
	}
}

func TestKeyframeInOutputRejectsMalformedOutput(t *testing.T) {
	t.Parallel()

	for _, output := range []string{`{"frames": [`, `{"frames": {}}`, ``} {
		if _, _, err := keyframeInOutput([]byte(output), time.Second); !errors.Is(err, domain.ErrUnsupported) {
			t.Errorf("keyframeInOutput(%q) error = %v, want ErrUnsupported", output, err)
		}
	}
}

func TestReadInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		position time.Duration
		window   time.Duration
		want     string
	}{
		{position: 21 * time.Second, window: 10 * time.Second, want: "+11.000000%+11.000000"},
		{position: 5 * time.Second, window: 10 * time.Second, want: "+0.000000%+6.000000"},
		{position: 2500 * time.Millisecond, window: time.Second, want: "+1.500000%+2.000000"},
		{position: 90*time.Minute + 1234567*time.Nanosecond, window: 60 * time.Second, want: "+5340.001234%+61.000000"},
	}

	for _, test := range tests {
		if got := readInterval(test.position, test.window); got != test.want {
			t.Errorf("readInterval(%s, %s) = %q, want %q", test.position, test.window, got, test.want)
		}
	}
}

func TestKeyframeAtOrBeforeSearchesWindowsThenFallsBackToZero(t *testing.T) {
	tsOutput := string(readFixture(t, "probe-keyframes-ts.json"))
	noneOutput := string(readFixture(t, "probe-keyframes-none.json"))

	tests := []struct {
		name      string
		responses map[string]string
		position  time.Duration
		want      time.Duration
		wantCalls []string
	}{
		{
			name:      "found in first window",
			responses: map[string]string{"+11.000000%+11.000000": tsOutput, fakeDefaultResponse: noneOutput},
			position:  21 * time.Second,
			want:      21440*time.Millisecond - tsStart,
			wantCalls: []string{"+11.000000%+11.000000"},
		},
		{
			name:      "found in wide window",
			responses: map[string]string{"+0.000000%+22.000000": tsOutput, fakeDefaultResponse: noneOutput},
			position:  21 * time.Second,
			want:      21440*time.Millisecond - tsStart,
			wantCalls: []string{"+11.000000%+11.000000", "+0.000000%+22.000000"},
		},
		{
			name:      "nothing found",
			responses: map[string]string{fakeDefaultResponse: noneOutput},
			position:  100 * time.Second,
			wantCalls: []string{"+90.000000%+11.000000", "+40.000000%+61.000000"},
		},
		{
			name:      "first window already reaches the start",
			responses: map[string]string{fakeDefaultResponse: noneOutput},
			position:  5 * time.Second,
			wantCalls: []string{"+0.000000%+6.000000"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake, calls := fakeResponder(t, test.responses)
			path := filepath.Join(t.TempDir(), "clip.ts")

			got, err := newTestProber(t, fake.executable).KeyframeAtOrBefore(t.Context(), path, test.position)
			if err != nil {
				t.Fatalf("KeyframeAtOrBefore: %v", err)
			}

			if got != test.want {
				t.Errorf("keyframe = %s, want %s", got, test.want)
			}

			assertKeyframeCalls(t, calls(), path, test.wantCalls)
		})
	}
}

func assertKeyframeCalls(t *testing.T, recorded [][]string, path string, intervals []string) {
	t.Helper()

	if len(recorded) != len(intervals) {
		t.Fatalf("got %d calls, want %d: %q", len(recorded), len(intervals), recorded)
	}

	for index, interval := range intervals {
		want := []string{
			"-v", "error", "-hide_banner", "-select_streams", "V:0", "-skip_frame", "nokey",
			"-show_entries", "frame=key_frame,pts_time,best_effort_timestamp_time:format=start_time",
			"-read_intervals", interval, "-print_format", "json",
			"-format_whitelist", wantDemuxersWithImages, "-protocol_whitelist", "file", "file:" + path,
		}
		if !slices.Equal(recorded[index], want) {
			t.Errorf("call %d = %q, want %q", index, recorded[index], want)
		}
	}
}

func TestKeyframeAtOrBeforeStartReturnsZeroWithoutFFprobe(t *testing.T) {
	t.Parallel()

	prober := newTestProber(t, filepath.Join(t.TempDir(), "no-such-ffprobe"))
	path := filepath.Join(t.TempDir(), "clip.mkv")

	for _, position := range []time.Duration{0, -time.Second} {
		got, err := prober.KeyframeAtOrBefore(t.Context(), path, position)
		if err != nil || got != 0 {
			t.Errorf("KeyframeAtOrBefore(%s) = %s, %v, want 0, nil", position, got, err)
		}
	}
}

func TestKeyframeAtOrBeforeReportsFailures(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T) *Prober
		path  func(t *testing.T) string
		want  error
	}{
		{
			name:  "relative path",
			setup: func(t *testing.T) *Prober { return newTestProber(t, "ffprobe-that-is-never-started") },
			path:  func(*testing.T) string { return "clip.mkv" },
			want:  domain.ErrInvalid,
		},
		{
			name:  "ffprobe fails",
			setup: func(t *testing.T) *Prober { return newTestProber(t, fakeTool(t, fakeModeFail).executable) },
			want:  domain.ErrUnsupported,
		},
		{
			name: "malformed output",
			setup: func(t *testing.T) *Prober {
				fake, _ := fakeResponder(t, map[string]string{fakeDefaultResponse: "["})

				return newTestProber(t, fake.executable)
			},
			want: domain.ErrUnsupported,
		},
		{
			name: "timeout",
			setup: func(t *testing.T) *Prober {
				prober := newTestProber(t, fakeTool(t, fakeModeHang).executable)
				prober.keyframeTimeout = 200 * time.Millisecond

				return prober
			},
			want: context.DeadlineExceeded,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "clip.mkv")
			if test.path != nil {
				path = test.path(t)
			}

			_, err := test.setup(t).KeyframeAtOrBefore(t.Context(), path, 21*time.Second)
			if !errors.Is(err, test.want) {
				t.Errorf("error = %v, want %v", err, test.want)
			}
		})
	}
}
