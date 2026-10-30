package ffmpeg

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain"
)

const (
	// readyPollInterval is how often Ready looks for the first segment. FFmpeg needs seconds
	// of input for a 6 s segment, so polling adds little delay and needs no file watcher.
	// The temp_file flag makes every file the playlist lists complete.
	readyPollInterval = 150 * time.Millisecond
	// stopTimeout bounds how long Stop waits for the killed process. It exceeds
	// processWaitDelay, after which Wait returns even when a stray child holds the pipes.
	stopTimeout = processWaitDelay + 5*time.Second
	// maxPlaylistReadBytes bounds what Ready reads of the playlist; the first segment is
	// listed within its first lines.
	maxPlaylistReadBytes = 64 << 10
	// ffmpegToolName names the ffmpeg executable in errors.
	ffmpegToolName = "ffmpeg"
	// inheritedInputPath names the input FFmpeg inherits on Linux: the first entry of
	// exec.Cmd.ExtraFiles becomes descriptor 3. Opening it reopens exactly the file the
	// producer opened inside the library root, seekable and independent of any later rename
	// or symlink swap of the path (ADR 0022).
	inheritedInputPath = "/proc/self/fd/3"
	// playlistCommentPrefix starts the tags and comments of a playlist; every other
	// non-empty line is a segment URI.
	playlistCommentPrefix = "#"
)

// errNoSegment reports an FFmpeg run that exited cleanly without writing a segment, for
// example because the start position lies beyond the end of the file.
var errNoSegment = errors.New("ffmpeg ended without writing a segment")

// InputOpener opens a media file inside its library root through os.Root. Implemented by
// adapters/fs/libraryfs (FileSystem.Open); errors wrap the domain errors of that method.
type InputOpener interface {
	Open(root, relativePath string) (*os.File, error)
}

// ProducerOptions configures NewProducer; every field is required.
type ProducerOptions struct {
	// FFmpegPath is the ffmpeg executable, the configured tools.ffmpeg value resolved by
	// the caller.
	FFmpegPath string
	// Inputs opens the media file of a job on Linux, where FFmpeg reads it through an
	// inherited descriptor instead of the resolved path.
	Inputs InputOpener
	// Logger receives one warning per failed production.
	Logger *slog.Logger
}

// Producer runs the FFmpeg process of an HLS session (ADR 0015, ADR 0024). It implements
// the StreamProducer port of app/playback and is safe for concurrent use.
type Producer struct {
	ffmpeg tool
	inputs InputOpener
	logger *slog.Logger
	// inheritInput hands FFmpeg the input as an open descriptor instead of a path. Only
	// Linux offers /proc/self/fd; Windows keeps the resolved path (ADR 0022).
	inheritInput bool
	pollInterval time.Duration
	stopTimeout  time.Duration
}

// NewProducer returns a producer that runs the ffmpeg executable of options. Errors wrap
// domain.ErrInvalid for a missing option.
func NewProducer(options ProducerOptions) (*Producer, error) {
	switch {
	case options.FFmpegPath == "":
		return nil, fmt.Errorf("new stream producer: ffmpeg path is empty: %w", domain.ErrInvalid)
	case options.Inputs == nil:
		return nil, fmt.Errorf("new stream producer: input opener is nil: %w", domain.ErrInvalid)
	case options.Logger == nil:
		return nil, fmt.Errorf("new stream producer: logger is nil: %w", domain.ErrInvalid)
	}

	return &Producer{
		ffmpeg:       tool{name: ffmpegToolName, executable: options.FFmpegPath},
		inputs:       options.Inputs,
		logger:       options.Logger,
		inheritInput: runtime.GOOS == "linux",
		pollInterval: readyPollInterval,
		stopTimeout:  stopTimeout,
	}, nil
}

// Start launches FFmpeg with the HLSArguments of job. The process runs under ctx, the
// playback service's context, until it finishes, fails, ctx ends or the production is
// stopped. Errors wrap domain.ErrInvalid or domain.ErrUnsupported for a job HLSArguments
// rejects, the error of the input opener, domain.ErrUnavailable when FFmpeg cannot be
// started, or ctx.Err().
func (p *Producer) Start(ctx context.Context, job appplayback.Job) (appplayback.Production, error) {
	production, err := p.start(ctx, job)
	if err != nil {
		return nil, fmt.Errorf("start stream production of session %s: %w", job.SessionID, err)
	}

	return production, nil
}

func (p *Producer) start(ctx context.Context, job appplayback.Job) (*production, error) {
	inputPath := job.InputPath
	if p.inheritInput {
		inputPath = inheritedInputPath
	}

	arguments, err := HLSArguments(HLSJob{
		Plan: job.Plan, InputPath: inputPath, StartPosition: job.StartPosition, OutputDirectory: job.OutputDir,
	})
	if err != nil {
		return nil, err
	}

	inherited, err := p.inheritedFiles(job)
	if err != nil {
		return nil, err
	}

	// FFmpeg holds its own copy of an inherited descriptor once it started.
	defer closeAll(inherited)

	processContext, cancel := context.WithCancel(ctx)

	started, err := p.ffmpeg.start(processContext, arguments, processFiles{inherited: inherited})
	if err != nil {
		cancel()

		return nil, err
	}

	production := &production{
		sessionID:   job.SessionID,
		directory:   job.OutputDir,
		logger:      p.logger,
		interval:    p.pollInterval,
		stopTimeout: p.stopTimeout,
		cancel:      cancel,
		done:        make(chan struct{}),
	}

	go production.supervise(ctx, started)

	return production, nil
}

// inheritedFiles opens the job's input inside its library root when FFmpeg inherits it.
func (p *Producer) inheritedFiles(job appplayback.Job) ([]*os.File, error) {
	if !p.inheritInput {
		return nil, nil
	}

	input, err := p.inputs.Open(job.LibraryRoot, job.RelativePath)
	if err != nil {
		return nil, fmt.Errorf("open input: %w", err)
	}

	return []*os.File{input}, nil
}

func closeAll(files []*os.File) {
	for _, file := range files {
		closeQuietly(file)
	}
}

// production is one running FFmpeg process; it implements appplayback.Production.
type production struct {
	sessionID   string
	directory   string
	logger      *slog.Logger
	interval    time.Duration
	stopTimeout time.Duration
	// cancel kills the process.
	cancel context.CancelFunc
	// done is closed by supervise once the process ended and result is set.
	done chan struct{}
	// result is how the process ended; written once, before done is closed.
	result error
	// stopped is set by Stop, before the kill, so Ready never reports the kill's exit.
	stopped atomic.Bool
	// ready caches that a first segment was seen.
	ready atomic.Bool
}

// Ready implements appplayback.Production: it returns nil once the playlist lists a first
// segment, the end result once the process ended (an error wrapping
// appplayback.ErrProductionStopped after Stop) and ctx.Err() when ctx ends first.
func (p *production) Ready(ctx context.Context) error {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		if settled, err := p.settled(); settled {
			return err
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for the first segment of session %s: %w", p.sessionID, ctx.Err())
		case <-p.done:
		case <-ticker.C:
		}
	}
}

// settled reports whether Ready can answer now, and the answer.
func (p *production) settled() (bool, error) {
	if p.stopped.Load() {
		return true, p.stoppedError()
	}

	select {
	case <-p.done:
		return true, p.result
	default:
	}

	if p.ready.Load() || playlistListsSegment(p.directory) {
		p.ready.Store(true)

		return true, nil
	}

	return false, nil
}

// Stop implements appplayback.Production: it kills the process and waits for it to exit,
// at most stopTimeout. It is idempotent and safe to call concurrently with Ready.
func (p *production) Stop() {
	p.stopped.Store(true)
	p.cancel()

	select {
	case <-p.done:
		return
	default:
	}

	timer := time.NewTimer(p.stopTimeout)
	defer timer.Stop()

	select {
	case <-p.done:
	case <-timer.C:
		p.logger.Warn("ffmpeg did not exit in time after it was stopped",
			slog.String("session", p.sessionID), slog.Duration("timeout", p.stopTimeout))
	}
}

// Done implements appplayback.Production.
func (p *production) Done() <-chan struct{} {
	return p.done
}

func (p *production) stoppedError() error {
	return fmt.Errorf("session %s: %w", p.sessionID, appplayback.ErrProductionStopped)
}

// supervise waits for the process, records how it ended and closes done. ctx is the
// context Start ran under, used for logging only.
func (p *production) supervise(ctx context.Context, started *process) {
	waitErr := started.wait()
	p.result = p.outcome(ctx, started, waitErr)
	p.cancel()
	close(p.done)
}

// outcome turns the end of the process into Ready's answer and logs a failure once. A
// process ended by its context (Stop or the service shutting down) is no failure.
func (p *production) outcome(ctx context.Context, started *process, waitErr error) error {
	if contextErr := started.ctx.Err(); contextErr != nil {
		return fmt.Errorf("session %s: %w (%s)", p.sessionID, appplayback.ErrProductionStopped, contextErr.Error())
	}

	segmentsWritten := playlistListsSegment(p.directory)
	if waitErr == nil && segmentsWritten {
		return nil
	}

	failure := waitErr
	if failure == nil {
		failure = errNoSegment
	}

	p.logFailure(ctx, failure, started.stderrTail(), segmentsWritten)

	return fmt.Errorf("stream production of session %s: %w", p.sessionID, failure)
}

// logFailure writes the one warning of a failed production. The task asks for FFmpeg's
// stderr tail at warn level (ADR 0024): it is the only record of why a transcode broke. It
// may contain the media path; logs stay on the server.
func (p *production) logFailure(ctx context.Context, failure error, stderrTail string, segmentsWritten bool) {
	exitCode := 0

	var exit *exitError
	if errors.As(failure, &exit) {
		exitCode = exit.code
	}

	p.logger.LogAttrs(ctx, slog.LevelWarn, "ffmpeg stream production failed",
		slog.String("session", p.sessionID),
		slog.Int("exit_code", exitCode),
		slog.Bool("segments_written", segmentsWritten),
		slog.String("error", failure.Error()),
		slog.String("stderr", stderrTail))
}

// playlistListsSegment reports whether the playlist in directory lists a segment; a missing
// or unreadable playlist lists none yet. It reads through os.Root because that opens files
// with FILE_SHARE_DELETE on Windows: FFmpeg replaces the playlist by renaming its temporary
// file over it, which fails while a reader holds it open without that share mode.
func playlistListsSegment(directory string) bool {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return false
	}

	defer closeQuietly(root)

	playlist, err := root.Open(hlsPlaylistName)
	if err != nil {
		return false
	}

	defer closeQuietly(playlist)

	return listsSegment(io.LimitReader(playlist, maxPlaylistReadBytes))
}

// listsSegment reports whether an HLS playlist lists at least one segment URI.
func listsSegment(playlist io.Reader) bool {
	lines := bufio.NewScanner(playlist)

	for lines.Scan() {
		line := strings.TrimSpace(lines.Text())
		if line != "" && !strings.HasPrefix(line, playlistCommentPrefix) {
			return true
		}
	}

	return false
}

// closeQuietly closes a handle opened for reading; a close error loses no data.
func closeQuietly(closer io.Closer) {
	_ = closer.Close()
}
