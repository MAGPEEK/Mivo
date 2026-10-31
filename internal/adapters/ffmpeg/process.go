package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/magpeek/mivo/internal/domain"
)

const (
	// maxOutputBytes bounds the standard output kept from a tool. ffprobe's JSON for a file
	// with dozens of streams stays far below it; more means a broken or hostile file.
	maxOutputBytes = 8 << 20
	// maxStderrTailBytes bounds the error output kept for logs; FFmpeg prints the cause
	// last, so the tail is the useful part.
	maxStderrTailBytes = 4 << 10
	// processWaitDelay is how long Wait waits for the output pipes after the process exited
	// or was killed, in case a stray child process still holds them open.
	processWaitDelay = 5 * time.Second
	// truncatedMarker precedes an error output whose beginning was dropped.
	truncatedMarker = "[truncated] "
	// exitCodeKey and stderrKey name the log attributes of a tool's unsuccessful exit.
	exitCodeKey = "exit_code"
	stderrKey   = "stderr"
)

// errOutputTooLarge reports a tool whose standard output exceeded maxOutputBytes.
var errOutputTooLarge = errors.New("output exceeds the size limit")

// tool is one FFmpeg executable (ffmpeg or ffprobe). The executable comes from the
// configuration (tools.ffmpeg, tools.ffprobe), never from PATH assumptions of this package.
type tool struct {
	// name identifies the tool in errors and logs.
	name       string
	executable string
}

// process is a started tool. wait must be called exactly once, on the goroutine that called
// start (childLifetime pins that goroutine to its OS thread on Linux).
type process struct {
	tool tool
	// ctx is the context the process runs under; wait reports it as the reason when it
	// ended the process.
	ctx      context.Context
	command  *exec.Cmd
	stderr   *tailBuffer
	lifetime *childLifetime
}

// exitError reports a tool that ran and exited unsuccessfully. Its message names the tool
// and the exit code only: FFmpeg's error output contains absolute paths and arbitrary bytes
// from the file, so only this package logs the bounded tail (exitAttributes).
type exitError struct {
	tool   string
	code   int
	stderr string
}

func (e *exitError) Error() string {
	return fmt.Sprintf("%s exited with code %d", e.tool, e.code)
}

// stderrTail returns the bounded tail of the tool's error output.
func (e *exitError) stderrTail() string {
	return e.stderr
}

// exitAttributes returns the exit code and the bounded stderr tail (at most 4 KiB) of err for
// a log record when err comes from a tool of this package that exited unsuccessfully, and
// nothing for any other error. The tail never leaves this package except into the server log
// (ADR 0024 Notes M2-39): it can contain absolute media paths, revealing the library roots,
// and arbitrary bytes taken from the media file, so no error, problem response or client
// message may carry it.
func exitAttributes(err error) []slog.Attr {
	var exit *exitError
	if !errors.As(err, &exit) {
		return nil
	}

	return []slog.Attr{slog.Int(exitCodeKey, exit.code), slog.String(stderrKey, exit.stderrTail())}
}

// output runs the tool to completion and returns its standard output. The process is
// killed when ctx ends (the error then wraps ctx.Err()) or when the output exceeds
// maxOutputBytes (errOutputTooLarge). A tool that cannot be started wraps
// domain.ErrUnavailable; an unsuccessful exit is an *exitError.
func (t tool) output(ctx context.Context, arguments []string) ([]byte, error) {
	runContext, cancel := context.WithCancel(ctx)
	defer cancel()

	stdout := &limitedBuffer{limit: maxOutputBytes, exceeded: cancel}

	started, err := t.start(runContext, arguments, processFiles{stdout: stdout})
	if err != nil {
		return nil, err
	}

	waitErr := started.wait()

	// When WaitDelay expires, Wait returns while the copy goroutine may still run, so the
	// overflow flag is atomic. stdout.data is read only after a nil waitErr, which means the
	// copy goroutine has finished (an expired WaitDelay always yields an error).
	if stdout.overflowed.Load() {
		return nil, fmt.Errorf("run %s: %w (%d bytes)", t.name, errOutputTooLarge, maxOutputBytes)
	}

	if waitErr != nil {
		return nil, waitErr
	}

	return stdout.data.Bytes(), nil
}

// processFiles are the files a started tool gets besides its error output.
type processFiles struct {
	// stdout receives the standard output; nil discards it.
	stdout io.Writer
	// inherited become the descriptors 3, 4, … of the process (unix only). The caller keeps
	// them and may close them once start returned: the process has its own copies.
	inherited []*os.File
}

// start starts the tool without waiting for it; long-running jobs (the HLS producer) use it
// directly. The process is killed when ctx ends, and with the server where the system allows
// it (childLifetime). The caller must call wait on the same goroutine.
func (t tool) start(ctx context.Context, arguments []string, files processFiles) (*process, error) {
	// The executable is the configured tool path and the arguments are a slice: no shell
	// is involved, so no argument can inject a command.
	command := exec.CommandContext(ctx, t.executable, arguments...) //nolint:gosec // see above

	stderr := newTailBuffer(maxStderrTailBytes)
	// A nil Stdin reads from the null device, so a tool never waits for console input.
	command.Stdin = nil
	command.Stdout = files.stdout
	command.Stderr = stderr
	command.ExtraFiles = files.inherited
	command.WaitDelay = processWaitDelay

	lifetime, err := bindChildLifetime(command)
	if err != nil {
		return nil, fmt.Errorf("start %s: %w: %w", t.name, domain.ErrUnavailable, err)
	}

	if err := command.Start(); err != nil {
		lifetime.release()

		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("start %s: %w", t.name, ctxErr)
		}

		return nil, fmt.Errorf("start %s: %w: %w", t.name, domain.ErrUnavailable, err)
	}

	if err := lifetime.adopt(command.Process); err != nil {
		// A tool that would outlive a crashed server is not run on.
		abandon(command)
		lifetime.release()

		return nil, fmt.Errorf("start %s: %w: %w", t.name, domain.ErrUnavailable, err)
	}

	return &process{tool: t, ctx: ctx, command: command, stderr: stderr, lifetime: lifetime}, nil
}

// abandon kills a started command and reaps it; its outcome no longer matters.
func abandon(command *exec.Cmd) {
	_ = command.Process.Kill()
	_ = command.Wait()
}

// wait waits for the process to end. A process stopped by its context reports ctx.Err(),
// whatever exit code the kill produced.
func (p *process) wait() error {
	err := p.command.Wait()
	p.lifetime.release()

	if ctxErr := p.ctx.Err(); ctxErr != nil {
		return fmt.Errorf("run %s: %w", p.tool.name, ctxErr)
	}

	if err == nil {
		return nil
	}

	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return &exitError{tool: p.tool.name, code: exit.ExitCode(), stderr: p.stderr.String()}
	}

	return fmt.Errorf("run %s: %w", p.tool.name, err)
}

// stderrTail returns the bounded tail of the error output written so far.
func (p *process) stderrTail() string {
	return p.stderr.String()
}

// limitedBuffer collects output up to limit bytes. The first write beyond it fails, which
// makes os/exec close the pipe, and calls exceeded so the process is killed as well.
type limitedBuffer struct {
	data       bytes.Buffer
	limit      int
	exceeded   func()
	overflowed atomic.Bool
}

func (b *limitedBuffer) Write(chunk []byte) (int, error) {
	if b.data.Len()+len(chunk) > b.limit {
		b.overflowed.Store(true)
		if b.exceeded != nil {
			b.exceeded()
		}

		return 0, errOutputTooLarge
	}

	written, err := b.data.Write(chunk)
	if err != nil {
		return written, fmt.Errorf("buffer output: %w", err)
	}

	return written, nil
}

// tailBuffer is an io.Writer that keeps only the last limit bytes written to it. It is safe
// for concurrent use, so a long-running process can be inspected while it writes.
type tailBuffer struct {
	mutex     sync.Mutex
	limit     int
	data      []byte
	truncated bool
}

func newTailBuffer(limit int) *tailBuffer {
	return &tailBuffer{limit: limit}
}

func (b *tailBuffer) Write(chunk []byte) (int, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.data = append(b.data, chunk...)

	if excess := len(b.data) - b.limit; excess > 0 {
		// Start at a rune boundary so the tail never begins with half a UTF-8 character
		// (FFmpeg prints localised file names).
		for excess < len(b.data) && !utf8.RuneStart(b.data[excess]) {
			excess++
		}

		b.data = append(b.data[:0], b.data[excess:]...)
		b.truncated = true
	}

	return len(chunk), nil
}

// String returns the kept output, trimmed and marked when earlier output was dropped.
func (b *tailBuffer) String() string {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	text := strings.TrimSpace(string(b.data))
	if b.truncated {
		return truncatedMarker + text
	}

	return text
}
