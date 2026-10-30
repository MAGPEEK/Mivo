package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/magpeek/mivo/internal/domain"
)

// killDeadline bounds how long a killed fake may take to be reaped; far below the fake's
// hang duration, so passing proves the kill.
const killDeadline = 30 * time.Second

func TestOutputReturnsStandardOutput(t *testing.T) {
	fake, _ := fakeResponder(t, map[string]string{fakeDefaultResponse: "hello 日本"})

	stdout, err := fake.output(t.Context(), []string{"-version"})
	if err != nil {
		t.Fatalf("output: %v", err)
	}

	if string(stdout) != "hello 日本" {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestOutputPassesArgumentsVerbatim(t *testing.T) {
	fake, calls := fakeResponder(t, map[string]string{})
	arguments := []string{"-v", "error", "file:-dash name.mkv", "a b", "Ümlaut ñ 日本", "$HOME", "x;y|z", ""}

	if _, err := fake.output(t.Context(), arguments); err != nil {
		t.Fatalf("output: %v", err)
	}

	recorded := calls()
	if len(recorded) != 1 || !slices.Equal(recorded[0], arguments) {
		t.Errorf("recorded = %q, want %q", recorded, arguments)
	}
}

func TestOutputReportsExitCodeWithoutStderr(t *testing.T) {
	fake := fakeTool(t, fakeModeFail)

	_, err := fake.output(t.Context(), nil)

	var exit *exitError
	if !errors.As(err, &exit) {
		t.Fatalf("error = %v, want *exitError", err)
	}

	if exit.code != fakeFailExitCode {
		t.Errorf("exit code = %d, want %d", exit.code, fakeFailExitCode)
	}

	message := err.Error()
	if !strings.Contains(message, "fakeprobe") || !strings.Contains(message, "code 3") {
		t.Errorf("error %q lacks tool name or exit code", message)
	}

	if strings.Contains(message, "Invalid data") || strings.Contains(message, "first line") {
		t.Errorf("error %q leaks stderr", message)
	}

	if tail := exit.stderrTail(); tail != strings.TrimSpace(fakeFailStderr) {
		t.Errorf("stderr tail = %q, want %q", tail, strings.TrimSpace(fakeFailStderr))
	}
}

func TestOutputKillsProcessWhenContextEnds(t *testing.T) {
	const stopAfter = 200 * time.Millisecond

	tests := []struct {
		name    string
		context func(t *testing.T) context.Context
		want    error
	}{
		{
			name: "deadline",
			context: func(t *testing.T) context.Context {
				ctx, cancel := context.WithTimeout(t.Context(), stopAfter)
				t.Cleanup(cancel)

				return ctx
			},
			want: context.DeadlineExceeded,
		},
		{
			name: "cancel",
			context: func(t *testing.T) context.Context {
				ctx, cancel := context.WithCancel(t.Context())
				time.AfterFunc(stopAfter, cancel)
				t.Cleanup(cancel)

				return ctx
			},
			want: context.Canceled,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := fakeTool(t, fakeModeHang)
			started := time.Now()

			_, err := fake.output(test.context(t), nil)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}

			if !strings.Contains(err.Error(), "fakeprobe") {
				t.Errorf("error %q lacks the tool name", err)
			}

			if elapsed := time.Since(started); elapsed > killDeadline {
				t.Errorf("returned after %s; the process was not killed", elapsed)
			}
		})
	}
}

func TestOutputRejectsOversizedOutput(t *testing.T) {
	fake := fakeTool(t, fakeModeFloodStdout)
	started := time.Now()

	_, err := fake.output(t.Context(), nil)
	if !errors.Is(err, errOutputTooLarge) {
		t.Fatalf("error = %v, want errOutputTooLarge", err)
	}

	if elapsed := time.Since(started); elapsed > killDeadline {
		t.Errorf("returned after %s", elapsed)
	}
}

func TestOutputKeepsBoundedStderrTail(t *testing.T) {
	fake := fakeTool(t, fakeModeFloodStderr)

	_, err := fake.output(t.Context(), nil)

	var exit *exitError
	if !errors.As(err, &exit) {
		t.Fatalf("error = %v, want *exitError", err)
	}

	tail := exit.stderrTail()
	if len(tail) > maxStderrTailBytes+len(truncatedMarker) {
		t.Errorf("tail has %d bytes, limit %d", len(tail), maxStderrTailBytes)
	}

	if !strings.HasPrefix(tail, truncatedMarker) || !utf8.ValidString(tail) {
		t.Errorf("tail is not a marked, valid UTF-8 tail: %q…", tail[:min(len(tail), 40)])
	}
}

func TestOutputReportsMissingExecutableAsUnavailable(t *testing.T) {
	t.Parallel()

	missing := tool{name: "ffprobe", executable: filepath.Join(t.TempDir(), "no-such-ffprobe")}

	_, err := missing.output(t.Context(), nil)
	if !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}

	if !strings.Contains(err.Error(), "ffprobe") {
		t.Errorf("error %q lacks the tool name", err)
	}
}

func TestOutputWithEndedContextIsNotUnavailable(t *testing.T) {
	fake := fakeTool(t, fakeModeHang)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := fake.output(ctx, nil)
	if !errors.Is(err, context.Canceled) || errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("error = %v, want context.Canceled only", err)
	}
}

func TestStartedProcessStopsWithItsContext(t *testing.T) {
	fake := fakeTool(t, fakeModeHang)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	process, err := fake.start(ctx, nil, processFiles{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	cancel()

	if err := process.wait(); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait = %v, want context.Canceled", err)
	}

	if tail := process.stderrTail(); tail != "" {
		t.Errorf("stderr tail = %q, want empty", tail)
	}
}

func TestTailBufferKeepsLastBytesAtRuneBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		limit  int
		writes []string
		want   string
	}{
		{name: "empty", limit: 8, want: ""},
		{name: "fits", limit: 8, writes: []string{"ab", "cd"}, want: "abcd"},
		{name: "trims whitespace", limit: 8, writes: []string{" ab\n"}, want: "ab"},
		{name: "drops head", limit: 4, writes: []string{"abc", "defg"}, want: truncatedMarker + "defg"},
		// "xééé" is 7 bytes; the cut at byte 3 already falls on the lead byte of an "é".
		{name: "cut on rune start", limit: 4, writes: []string{"xééé"}, want: truncatedMarker + "éé"},
		// "éé" is 4 bytes; the cut at byte 1 falls inside the first "é" and moves forward to
		// the next rune, keeping fewer bytes than the limit rather than half a character.
		{name: "cut inside rune", limit: 3, writes: []string{"éé"}, want: truncatedMarker + "é"},
		{name: "many small writes", limit: 3, writes: []string{"a", "b", "c", "d", "e"}, want: truncatedMarker + "cde"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			buffer := newTailBuffer(test.limit)

			for _, write := range test.writes {
				if written, err := buffer.Write([]byte(write)); err != nil || written != len(write) {
					t.Fatalf("Write(%q) = %d, %v", write, written, err)
				}
			}

			if got := buffer.String(); got != test.want {
				t.Errorf("String() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestLimitedBufferRejectsWritesBeyondLimit(t *testing.T) {
	t.Parallel()

	exceeded := false
	buffer := &limitedBuffer{limit: 4, exceeded: func() { exceeded = true }}

	if _, err := buffer.Write([]byte("abcd")); err != nil {
		t.Fatalf("write within limit: %v", err)
	}

	if _, err := buffer.Write([]byte("e")); !errors.Is(err, errOutputTooLarge) {
		t.Fatalf("write beyond limit = %v, want errOutputTooLarge", err)
	}

	if !exceeded || !buffer.overflowed.Load() || buffer.data.String() != "abcd" {
		t.Errorf("exceeded = %t, overflowed = %t, data = %q", exceeded, buffer.overflowed.Load(), buffer.data.String())
	}
}

func TestErrorOutput(t *testing.T) {
	t.Parallel()

	exit := &exitError{tool: "ffprobe", code: 1, stderr: "file:/media/x.mkv: Invalid data"}

	tests := []struct {
		name   string
		err    error
		want   string
		wantOK bool
	}{
		{name: "exit error", err: exit, want: exit.stderr, wantOK: true},
		{name: "wrapped exit error", err: fmt.Errorf("probe x.mkv: %w: %w", domain.ErrUnsupported, exit), want: exit.stderr, wantOK: true},
		{name: "other error", err: context.DeadlineExceeded},
		{name: "nil", err: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, ok := ErrorOutput(test.err)
			if got != test.want || ok != test.wantOK {
				t.Errorf("ErrorOutput(%v) = %q, %t, want %q, %t", test.err, got, ok, test.want, test.wantOK)
			}
		})
	}
}
