package ffmpeg

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
)

const (
	// fixtureCommentPrefix starts the header line of a recording that says how it was made.
	fixtureCommentPrefix = "#"
	// progressBlockLines is the number of lines FFmpeg 9.0.2 prints per block for one video
	// stream.
	progressBlockLines = 12
	// progressDeadline bounds the wait for the fake's progress block.
	progressDeadline = 10 * time.Second
)

// steppingClock is a clock tests move by hand.
type steppingClock struct {
	current time.Time
}

func (c *steppingClock) now() time.Time { return c.current }

func (c *steppingClock) advance(step time.Duration) { c.current = c.current.Add(step) }

// unthrottledTracker publishes every block, so a test sees what each block parsed to.
func unthrottledTracker() *progressTracker {
	tracker := newProgressTracker()
	tracker.interval = 0

	return tracker
}

func writeProgress(t *testing.T, tracker *progressTracker, text string) {
	t.Helper()

	written, err := tracker.Write([]byte(text))
	if err != nil || written != len(text) {
		t.Fatalf("Write = %d, %v, want %d, nil", written, err, len(text))
	}
}

// recordedProgress returns the lines of a recording without its header.
func recordedProgress(t *testing.T, name string) []string {
	t.Helper()

	var lines []string

	for line := range strings.Lines(string(readFixture(t, name))) {
		if !strings.HasPrefix(line, fixtureCommentPrefix) {
			lines = append(lines, line)
		}
	}

	return lines
}

func TestProgressTrackerParsesRecordedOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fixture string
		// blocks limits the input to the first blocks; 0 feeds the whole recording.
		blocks int
		want   appplayback.Progress
	}{
		{
			name: "software 1080p transcode, first block", fixture: "progress-9.0.2-transcode-1080p.txt", blocks: 1,
			want: appplayback.Progress{OutputTime: 2623855 * time.Microsecond, Speed: 5.18},
		},
		{
			name: "software 1080p transcode, running", fixture: "progress-9.0.2-transcode-1080p.txt", blocks: 5,
			want: appplayback.Progress{OutputTime: 4342132 * time.Microsecond, Speed: 1.69, FPS: 16.38},
		},
		{
			// The blocks before the end report out_time_us=N/A and speed=N/A: the last known
			// values stay.
			name: "software 1080p transcode, flushing", fixture: "progress-9.0.2-transcode-1080p.txt", blocks: 8,
			want: appplayback.Progress{OutputTime: 4342132 * time.Microsecond, Speed: 1.69, FPS: 19.22},
		},
		{
			name: "software 1080p transcode, ended", fixture: "progress-9.0.2-transcode-1080p.txt",
			want: appplayback.Progress{OutputTime: 40 * time.Second, Speed: 0.768, FPS: 19.21},
		},
		{
			// A remux reports no frame rate and pads the speed: "speed= 150x".
			name: "remux", fixture: "progress-9.0.2-remux.txt",
			want: appplayback.Progress{OutputTime: time.Minute, Speed: 150},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			lines := recordedProgress(t, test.fixture)
			if test.blocks > 0 {
				lines = lines[:test.blocks*progressBlockLines]
			}

			tracker := unthrottledTracker()
			writeProgress(t, tracker, strings.Join(lines, ""))

			if got := tracker.snapshot(); got != test.want {
				t.Errorf("snapshot = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestProgressTrackerParsesLines(t *testing.T) {
	t.Parallel()

	known := appplayback.Progress{OutputTime: 3 * time.Second, Speed: 1.5, FPS: 24, BitRate: 2_000_000}

	tests := []struct {
		name string
		line string
		want appplayback.Progress
	}{
		{name: "output time", line: "out_time_us=7500000", want: with(known, func(p *appplayback.Progress) { p.OutputTime = 7500 * time.Millisecond })},
		{name: "speed", line: "speed=0.998x", want: with(known, func(p *appplayback.Progress) { p.Speed = 0.998 })},
		{name: "padded speed", line: "speed=   1x", want: with(known, func(p *appplayback.Progress) { p.Speed = 1 })},
		{name: "frame rate", line: "fps=59.94", want: with(known, func(p *appplayback.Progress) { p.FPS = 59.94 })},
		{name: "bit rate", line: "bitrate=1234.5kbits/s", want: with(known, func(p *appplayback.Progress) { p.BitRate = 1_234_500 })},
		{name: "padded bit rate", line: "bitrate=  96.0kbits/s", want: with(known, func(p *appplayback.Progress) { p.BitRate = 96_000 })},
		{name: "carriage return", line: "speed=2.5x\r", want: with(known, func(p *appplayback.Progress) { p.Speed = 2.5 })},
		{name: "speed not available", line: "speed=N/A", want: known},
		{name: "output time not available", line: "out_time_us=N/A", want: known},
		{name: "bit rate not available", line: "bitrate=N/A", want: known},
		{name: "negative output time", line: "out_time_us=-9223372036854775808", want: known},
		{name: "output time out of range", line: "out_time_us=99999999999999999999", want: known},
		{name: "output time overflowing a duration", line: "out_time_us=9223372036854775807", want: known},
		{name: "speed without unit", line: "speed=1.5", want: known},
		{name: "speed not a number", line: "speed=fastx", want: known},
		{name: "negative speed", line: "speed=-1x", want: known},
		{name: "infinite speed", line: "speed=Infx", want: known},
		{name: "speed not a number value", line: "speed=NaNx", want: known},
		{name: "frame rate not a number", line: "fps=many", want: known},
		{name: "bit rate without unit", line: "bitrate=1234.5", want: known},
		{name: "bit rate beyond an integer", line: "bitrate=1e300kbits/s", want: known},
		{name: "total size is not part of the snapshot", line: "total_size=1048576", want: known},
		{name: "unknown key", line: "stream_0_0_q=28.0", want: known},
		{name: "no separator", line: "speed 2x", want: known},
		{name: "empty key", line: "=2x", want: known},
		{name: "empty line", line: "", want: known},
		{name: "unknown progress state", line: "progress=maybe", want: known},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			tracker := unthrottledTracker()
			writeProgress(t, tracker,
				"out_time_us=3000000\nspeed=1.5x\nfps=24.00\nbitrate=2000.0kbits/s\nprogress=continue\n")

			if got := tracker.snapshot(); got != known {
				t.Fatalf("snapshot before the line = %+v, want %+v", got, known)
			}

			writeProgress(t, tracker, test.line+"\nprogress=continue\n")

			if got := tracker.snapshot(); got != test.want {
				t.Errorf("snapshot after %q = %+v, want %+v", test.line, got, test.want)
			}
		})
	}
}

// with returns a copy of progress changed by change.
func with(progress appplayback.Progress, change func(*appplayback.Progress)) appplayback.Progress {
	change(&progress)

	return progress
}

func TestProgressTrackerIsEmptyBeforeTheFirstBlock(t *testing.T) {
	t.Parallel()

	tracker := newProgressTracker()

	if got := tracker.snapshot(); got != (appplayback.Progress{}) {
		t.Errorf("snapshot before any output = %+v, want zero", got)
	}

	// Values are published with the block they belong to, not line by line.
	writeProgress(t, tracker, "out_time_us=1000000\nspeed=2x\n")

	if got := tracker.snapshot(); got != (appplayback.Progress{}) {
		t.Errorf("snapshot of an unfinished block = %+v, want zero", got)
	}
}

func TestProgressTrackerCoalescesUpdatesToOnePerSecond(t *testing.T) {
	t.Parallel()

	clock := &steppingClock{current: time.Date(2026, time.November, 11, 12, 0, 0, 0, time.UTC)}
	tracker := newProgressTracker()
	tracker.now = clock.now

	steps := []struct {
		after     time.Duration
		speed     string
		state     string
		wantSpeed float64
	}{
		{after: 0, speed: "1x", state: progressStateContinue, wantSpeed: 1},
		{after: 500 * time.Millisecond, speed: "2x", state: progressStateContinue, wantSpeed: 1},
		{after: 499 * time.Millisecond, speed: "3x", state: progressStateContinue, wantSpeed: 1},
		{after: time.Millisecond, speed: "4x", state: progressStateContinue, wantSpeed: 4},
		// A block without a speed publishes the value of the blocks coalesced before it.
		{after: 900 * time.Millisecond, speed: "5x", state: progressStateContinue, wantSpeed: 4},
		{after: 100 * time.Millisecond, speed: "N/A", state: progressStateContinue, wantSpeed: 5},
		// The last block is published whenever it comes: nothing follows it.
		{after: time.Millisecond, speed: "6x", state: progressStateEnd, wantSpeed: 6},
	}

	for index, step := range steps {
		clock.advance(step.after)
		writeProgress(t, tracker, "speed="+step.speed+"\nprogress="+step.state+"\n")

		if got := tracker.snapshot().Speed; got != step.wantSpeed {
			t.Errorf("step %d: speed = %v, want %v", index, got, step.wantSpeed)
		}
	}
}

func TestProgressTrackerAcceptsAnyChunking(t *testing.T) {
	t.Parallel()

	text := strings.Join(recordedProgress(t, "progress-9.0.2-transcode-1080p.txt"), "")
	want := appplayback.Progress{OutputTime: 40 * time.Second, Speed: 0.768, FPS: 19.21}

	for _, chunkSize := range []int{1, 7, 4096} {
		tracker := unthrottledTracker()

		for chunk := range slices.Chunk([]byte(text), chunkSize) {
			writeProgress(t, tracker, string(chunk))
		}

		if got := tracker.snapshot(); got != want {
			t.Errorf("chunks of %d bytes: snapshot = %+v, want %+v", chunkSize, got, want)
		}
	}
}

func TestProgressTrackerDrainsOutputThatIsNoProgress(t *testing.T) {
	t.Parallel()

	tracker := unthrottledTracker()
	flood := strings.Repeat("x", fakeFloodChunk)

	// A line without an end must neither fail the write, which would close the pipe and block
	// FFmpeg, nor grow the buffer.
	for range 64 {
		writeProgress(t, tracker, flood)
	}

	if kept := cap(tracker.line); kept > maxProgressLineBytes {
		t.Errorf("the tracker keeps %d bytes of an endless line, want at most %d", kept, maxProgressLineBytes)
	}

	// The rest of the overlong line is dropped even when it looks like progress.
	writeProgress(t, tracker, "speed=9x\nspeed=2x\nprogress=continue\n")

	if got := tracker.snapshot(); got != (appplayback.Progress{Speed: 2}) {
		t.Errorf("snapshot after the flood = %+v, want speed 2 only", got)
	}
}

func TestProducerReportsTheProgressOfFFmpeg(t *testing.T) {
	argumentsPath := filepath.Join(t.TempDir(), "arguments.jsonl")
	t.Setenv(fakeArgumentsVariable, argumentsPath)

	test := newProducerTest(t, fakeModeHLSLive)
	production := test.start(t.Context(), t)

	reporter, reports := production.(appplayback.ProgressReporter)
	if !reports {
		t.Fatalf("%T does not implement appplayback.ProgressReporter", production)
	}

	if err := readyWithin(t, production, readyDeadline); err != nil {
		t.Fatalf("Ready: %v", err)
	}

	want := appplayback.Progress{OutputTime: 6 * time.Second, Speed: 2, FPS: 24, BitRate: 800_000}
	deadline := time.Now().Add(progressDeadline)

	for reporter.Progress() != want {
		if time.Now().After(deadline) {
			t.Fatalf("Progress = %+v after %s, want %+v", reporter.Progress(), progressDeadline, want)
		}

		time.Sleep(testPollInterval)
	}

	calls := readRecordedCalls(t, argumentsPath)
	if len(calls) != 1 || !slices.Equal(calls[0][:3], []string{"-progress", "pipe:1", "-nostats"}) {
		t.Errorf("arguments %q do not start with -progress pipe:1 -nostats", calls)
	}

	production.Stop()

	// The last snapshot stays readable after the process ended.
	if got := reporter.Progress(); got != want {
		t.Errorf("Progress after Stop = %+v, want %+v", got, want)
	}

	assertNoWarnings(t, test.logs)
}
