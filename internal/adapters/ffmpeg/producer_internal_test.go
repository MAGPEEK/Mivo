package ffmpeg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// The producer satisfies the port of app/playback.
var _ appplayback.StreamProducer = (*Producer)(nil)

const (
	testSessionID = "00000000-0000-4000-8000-000000000024"
	// testPollInterval keeps the unit tests fast; the fake delays its segment by
	// fakeSegmentDelay, so Ready still polls several times.
	testPollInterval = 20 * time.Millisecond
	// readyDeadline bounds a wait that must succeed; far above the fake's segment delay.
	readyDeadline = 30 * time.Second
	// notReadyWait is how long a test waits for a readiness that must not come: longer than
	// the fake's segment delay plus several polls.
	notReadyWait = 800 * time.Millisecond
	// concurrentCallers is the number of goroutines calling Ready and Stop at once.
	concurrentCallers = 4

	testMovieName    = "movie.mkv"
	testMovieContent = "the movie inside the library"
	testDecoyContent = "a decoy reached through the resolved path"

	warnLevel = "WARN"
)

// rootOpener opens inputs through os.Root, like libraryfs.FileSystem.Open.
type rootOpener struct{}

func (rootOpener) Open(root, relativePath string) (*os.File, error) {
	libraryRoot, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open root: %w", err)
	}

	file, openErr := libraryRoot.Open(relativePath)
	if err := errors.Join(openErr, libraryRoot.Close()); err != nil {
		return nil, fmt.Errorf("open %s: %w", relativePath, err)
	}

	return file, nil
}

// syncBuffer collects log output written by the producer's goroutines.
type syncBuffer struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (b *syncBuffer) Write(data []byte) (int, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	written, err := b.buffer.Write(data)
	if err != nil {
		return written, fmt.Errorf("buffer log: %w", err)
	}

	return written, nil
}

// warnings returns the decoded warn records written so far.
func (b *syncBuffer) warnings(t *testing.T) []map[string]any {
	t.Helper()

	var warnings []map[string]any

	for _, record := range b.records(t) {
		if record[slog.LevelKey] == warnLevel {
			warnings = append(warnings, record)
		}
	}

	return warnings
}

// records returns every decoded JSON log record written so far.
func (b *syncBuffer) records(t *testing.T) []map[string]any {
	t.Helper()

	b.mutex.Lock()
	text := b.buffer.String()
	b.mutex.Unlock()

	var records []map[string]any

	for line := range strings.Lines(text) {
		record := map[string]any{}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode log line %q: %v", line, err)
		}

		records = append(records, record)
	}

	return records
}

// producerTest is a producer that starts the fake tool in mode, and a job whose library
// holds testMovieName while InputPath names a decoy outside it.
type producerTest struct {
	producer *Producer
	logs     *syncBuffer
	job      appplayback.Job
}

func newProducerTest(t *testing.T, mode string) producerTest {
	t.Helper()

	fake := fakeTool(t, mode)
	logs := &syncBuffer{}

	producer, err := NewProducer(ProducerOptions{
		FFmpegPath: fake.executable,
		Inputs:     rootOpener{},
		Logger:     slog.New(slog.NewJSONHandler(logs, nil)),
	})
	if err != nil {
		t.Fatalf("NewProducer: %v", err)
	}

	producer.pollInterval = testPollInterval

	library, outside := t.TempDir(), t.TempDir()
	writeTestFile(t, library, testMovieName, testMovieContent)
	writeTestFile(t, outside, testMovieName, testDecoyContent)

	return producerTest{
		producer: producer,
		logs:     logs,
		job: appplayback.Job{
			SessionID: testSessionID,
			Plan: playback.Plan{
				Mode: playback.ModeRemux, Delivery: playback.DeliveryHLS,
				Streams: []playback.StreamDecision{
					{Kind: media.StreamKindVideo, SourceIndex: 0, Action: playback.ActionCopy, TargetCodec: "h264"},
					{Kind: media.StreamKindAudio, SourceIndex: 1, Action: playback.ActionCopy, TargetCodec: "aac"},
				},
			},
			LibraryRoot:  library,
			RelativePath: testMovieName,
			InputPath:    filepath.Join(outside, testMovieName),
			OutputDir:    t.TempDir(),
		},
	}
}

// start starts the job and stops the production when the test ends.
func (test producerTest) start(ctx context.Context, t *testing.T) appplayback.Production {
	t.Helper()

	production, err := test.producer.Start(ctx, test.job)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	t.Cleanup(production.Stop)

	return production
}

func writeTestFile(t *testing.T, directory, name, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func readyWithin(t *testing.T, production appplayback.Production, wait time.Duration) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), wait)
	defer cancel()

	return production.Ready(ctx)
}

func awaitDone(t *testing.T, production appplayback.Production) {
	t.Helper()

	select {
	case <-production.Done():
	case <-time.After(killDeadline):
		t.Fatalf("the process did not end within %s", killDeadline)
	}
}

// awaitFile waits until path exists.
func awaitFile(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(readyDeadline)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("%s did not appear within %s", filepath.Base(path), readyDeadline)
		}

		time.Sleep(testPollInterval)
	}
}

func isDone(production appplayback.Production) bool {
	select {
	case <-production.Done():
		return true
	default:
		return false
	}
}

func assertStopped(t *testing.T, err error) {
	t.Helper()

	if !errors.Is(err, appplayback.ErrProductionStopped) || !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Ready = %v, want ErrProductionStopped", err)
	}

	if _, isExit := stderrOf(err); isExit {
		t.Errorf("Ready after a stop carries the kill's exit error: %v", err)
	}
}

func assertNoWarnings(t *testing.T, logs *syncBuffer) {
	t.Helper()

	if warnings := logs.warnings(t); len(warnings) > 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
}

// assertOneFailureLog checks the single warn record of a failed production.
func assertOneFailureLog(t *testing.T, logs *syncBuffer, exitCode int, segmentsWritten bool) map[string]any {
	t.Helper()

	warnings := logs.warnings(t)
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want exactly 1: %v", len(warnings), warnings)
	}

	record := warnings[0]

	// JSON numbers decode as float64.
	if record["session"] != testSessionID || record["exit_code"] != float64(exitCode) ||
		record["segments_written"] != segmentsWritten {
		t.Errorf("warning = %v, want session %s, exit_code %d, segments_written %t",
			record, testSessionID, exitCode, segmentsWritten)
	}

	return record
}

func TestNewProducerRejectsIncompleteOptions(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)

	tests := []struct {
		name    string
		options ProducerOptions
	}{
		{name: "no ffmpeg", options: ProducerOptions{Inputs: rootOpener{}, Logger: logger}},
		{name: "no input opener", options: ProducerOptions{FFmpegPath: "ffmpeg", Logger: logger}},
		{name: "no logger", options: ProducerOptions{FFmpegPath: "ffmpeg", Inputs: rootOpener{}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := NewProducer(test.options); !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("NewProducer = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestProducerReadyWaitsForTheFirstSegment(t *testing.T) {
	test := newProducerTest(t, fakeModeHLSLive)
	production := test.start(t.Context(), t)

	if err := readyWithin(t, production, readyDeadline); err != nil {
		t.Fatalf("Ready: %v", err)
	}

	if _, err := os.Stat(filepath.Join(test.job.OutputDir, fakeSegmentName)); err != nil {
		t.Errorf("Ready returned before the segment exists: %v", err)
	}

	if isDone(production) {
		t.Fatal("Done is closed while the process runs")
	}

	// A second caller gets the answer at once.
	if err := readyWithin(t, production, readyDeadline); err != nil {
		t.Errorf("second Ready: %v", err)
	}

	production.Stop()

	if !isDone(production) {
		t.Fatal("Done is open after Stop returned")
	}

	assertStopped(t, production.Ready(t.Context()))
	assertNoWarnings(t, test.logs)
}

func TestProducerHandsTheLibraryFileToFFmpeg(t *testing.T) {
	test := newProducerTest(t, fakeModeHLSComplete)
	argumentsPath := filepath.Join(t.TempDir(), "arguments.jsonl")
	t.Setenv(fakeArgumentsVariable, argumentsPath)

	production := test.start(t.Context(), t)
	awaitDone(t, production)

	if err := production.Ready(t.Context()); err != nil {
		t.Fatalf("Ready after a complete run = %v, want nil", err)
	}

	// Linux closes the gap between Resolve and FFmpeg's open: FFmpeg gets the file the
	// producer opened inside the library root, whatever InputPath says. Elsewhere it opens
	// the resolved path (ADR 0022).
	wantInput, wantContent := fileProtocol+test.job.InputPath, testDecoyContent
	if runtime.GOOS == "linux" {
		wantInput, wantContent = fileProtocol+inheritedInputPath, testMovieContent
	}

	segment, err := os.ReadFile(filepath.Join(test.job.OutputDir, fakeSegmentName))
	if err != nil || string(segment) != wantContent {
		t.Errorf("FFmpeg read %q (%v), want %q", segment, err, wantContent)
	}

	calls := readRecordedCalls(t, argumentsPath)
	if len(calls) != 1 {
		t.Fatalf("recorded %d calls, want 1", len(calls))
	}

	input := slices.Index(calls[0], "-i")
	wantInputArguments := []string{"-format_whitelist", wantDemuxersWithImages, "-protocol_whitelist", "file", "-i", wantInput}

	if input < 4 || !slices.Equal(calls[0][input-4:input+2], wantInputArguments) {
		t.Errorf("arguments %q lack %q", calls[0], wantInputArguments)
	}

	assertNoWarnings(t, test.logs)
}

func TestProducerReportsAnExitBeforeTheFirstSegment(t *testing.T) {
	test := newProducerTest(t, fakeModeFail)
	production := test.start(t.Context(), t)

	err := readyWithin(t, production, readyDeadline)
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("exited with code %d", fakeFailExitCode)) {
		t.Fatalf("Ready = %v, want the exit code %d", err, fakeFailExitCode)
	}

	if errors.Is(err, appplayback.ErrProductionStopped) {
		t.Errorf("a failure is reported as a stop: %v", err)
	}

	if tail, ok := stderrOf(err); !ok || !strings.Contains(tail, "Invalid data") {
		t.Errorf("stderr tail = %q, %t, want the stderr tail", tail, ok)
	}

	if !isDone(production) {
		t.Error("Ready reported the exit before Done was closed")
	}

	record := assertOneFailureLog(t, test.logs, fakeFailExitCode, false)
	if stderr, _ := record["stderr"].(string); !strings.Contains(stderr, "Invalid data") {
		t.Errorf("warning lacks the stderr tail: %v", record)
	}
}

func TestProducerLogsAFailureAfterReadinessOnce(t *testing.T) {
	test := newProducerTest(t, fakeModeHLSFailAfterSegment)
	production := test.start(t.Context(), t)
	awaitDone(t, production)

	for range 2 {
		if err := production.Ready(t.Context()); err == nil || !strings.Contains(err.Error(), "exited with code") {
			t.Errorf("Ready after the failure = %v, want the exit error", err)
		}
	}

	// Stop after the exit changes nothing in the log.
	production.Stop()
	assertOneFailureLog(t, test.logs, fakeFailExitCode, true)
}

func TestProducerReportsACleanExitWithoutSegments(t *testing.T) {
	// The respond mode prints nothing and exits with 0, like FFmpeg on an empty selection.
	test := newProducerTest(t, fakeModeRespond)
	production := test.start(t.Context(), t)

	err := readyWithin(t, production, readyDeadline)
	if !errors.Is(err, errNoSegment) || errors.Is(err, appplayback.ErrProductionStopped) {
		t.Fatalf("Ready = %v, want errNoSegment", err)
	}

	// FFmpeg did exit, with code 0.
	assertOneFailureLog(t, test.logs, 0, false)
}

func TestProducerCompletesALostFinalPlaylistRename(t *testing.T) {
	test := newProducerTest(t, fakeModeHLSLostFinalRename)
	production := test.start(t.Context(), t)
	awaitDone(t, production)

	if err := production.Ready(t.Context()); err != nil {
		t.Fatalf("Ready after a clean end = %v, want nil", err)
	}

	playlist, err := os.ReadFile(filepath.Join(test.job.OutputDir, hlsPlaylistName))
	if err != nil || !strings.HasSuffix(string(playlist), fakePlaylistEnd) {
		t.Errorf("playlist = %q (%v), want the final playlist with #EXT-X-ENDLIST", playlist, err)
	}

	if _, err := os.Stat(filepath.Join(test.job.OutputDir, temporaryPlaylistName)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the temporary playlist is left: %v", err)
	}

	assertNoWarnings(t, test.logs)
}

func TestCompletePlaylist(t *testing.T) {
	t.Parallel()

	const (
		stalePlaylist = fakePlaylistHeader + fakePlaylistSegment
		finalPlaylist = stalePlaylist + fakePlaylistEnd
		// readerHold is how long a reader keeps the playlist open; the rename must wait.
		readerHold = 100 * time.Millisecond
	)

	t.Run("nothing to complete", func(t *testing.T) {
		t.Parallel()

		directory := t.TempDir()
		writeTestFile(t, directory, hlsPlaylistName, finalPlaylist)

		if err := completePlaylist(directory); err != nil {
			t.Fatalf("completePlaylist = %v, want nil", err)
		}

		assertFileContent(t, filepath.Join(directory, hlsPlaylistName), finalPlaylist)
	})

	t.Run("while a reader holds the playlist", func(t *testing.T) {
		t.Parallel()

		directory := t.TempDir()
		writeTestFile(t, directory, hlsPlaylistName, stalePlaylist)
		writeTestFile(t, directory, temporaryPlaylistName, finalPlaylist)

		// Windows refuses to replace a file another handle has open.
		reader, err := os.Open(filepath.Join(directory, hlsPlaylistName)) //nolint:gosec // test directory
		if err != nil {
			t.Fatalf("open the playlist: %v", err)
		}

		time.AfterFunc(readerHold, func() { _ = reader.Close() })

		if err := completePlaylist(directory); err != nil {
			t.Fatalf("completePlaylist = %v, want nil", err)
		}

		assertFileContent(t, filepath.Join(directory, hlsPlaylistName), finalPlaylist)
	})

	t.Run("rename keeps failing", func(t *testing.T) {
		t.Parallel()

		directory := t.TempDir()
		writeTestFile(t, directory, temporaryPlaylistName, finalPlaylist)

		// A non-empty directory under the playlist's name can never be replaced.
		if err := os.MkdirAll(filepath.Join(directory, hlsPlaylistName, "blocker"), 0o750); err != nil {
			t.Fatalf("create blocker: %v", err)
		}

		if err := completePlaylist(directory); err == nil {
			t.Fatal("completePlaylist = nil, want the rename error")
		}
	})
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()

	data, err := os.ReadFile(path) //nolint:gosec // test directory
	if err != nil || string(data) != want {
		t.Errorf("%s = %q (%v), want %q", filepath.Base(path), data, err, want)
	}
}

func TestProducerLogsNoExitCodeForAFailureThatIsNoExit(t *testing.T) {
	t.Parallel()

	logs := &syncBuffer{}
	failed := &production{sessionID: testSessionID, logger: slog.New(slog.NewJSONHandler(logs, nil))}

	failed.logFailure(t.Context(), fmt.Errorf("run ffmpeg: %w", exec.ErrWaitDelay), "", true)

	warnings := logs.warnings(t)
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(warnings), warnings)
	}

	if code, found := warnings[0]["exit_code"]; found {
		t.Errorf("warning carries exit_code %v for a failure that is no exit: %v", code, warnings[0])
	}

	if warnings[0]["session"] != testSessionID || warnings[0]["segments_written"] != true {
		t.Errorf("warning = %v", warnings[0])
	}
}

// TestProducerOutlivesTheGoroutineThatStartedIt: Linux sends the parent-death signal when the
// OS thread that started FFmpeg ends, and Go ends a thread when a goroutine locked to it
// returns. A caller doing that must not take the production with it (lifetime_linux.go).
func TestProducerOutlivesTheGoroutineThatStartedIt(t *testing.T) {
	// Go parks the main thread instead of ending it, and only one goroutine can park it, so
	// with two starting goroutines at least one thread really ends.
	const startingGoroutines = 2

	test := newProducerTest(t, fakeModeHLSLive)

	for range startingGoroutines {
		job := test.job
		job.OutputDir = t.TempDir()

		production := startOnEndingThread(t, test.producer, job)

		// The fake writes its segment only after fakeSegmentDelay, long after the starting
		// goroutine and its thread ended.
		if err := readyWithin(t, production, readyDeadline); err != nil {
			t.Fatalf("Ready = %v, want the production to survive its starting goroutine", err)
		}

		if isDone(production) {
			t.Error("the process ended with the goroutine that started it")
		}
	}
}

// startOnEndingThread starts job from a goroutine that ends locked to its OS thread, so the
// thread ends with it, and stops the production when the test ends.
func startOnEndingThread(t *testing.T, producer *Producer, job appplayback.Job) appplayback.Production {
	t.Helper()

	started := make(chan appplayback.Production, 1)

	go func() {
		// Never unlocked: the thread ends together with this goroutine.
		runtime.LockOSThread()

		production, err := producer.Start(t.Context(), job)
		if err != nil {
			t.Errorf("Start: %v", err)
		}

		started <- production
	}()

	production := <-started
	if production == nil {
		t.FailNow()
	}

	t.Cleanup(production.Stop)

	return production
}

func TestProducerReadyEndsWithItsContext(t *testing.T) {
	for _, mode := range []string{fakeModeHang, fakeModeHLSHeaderOnly} {
		t.Run(mode, func(t *testing.T) {
			test := newProducerTest(t, mode)
			production := test.start(t.Context(), t)

			// The header-only fake writes its playlist after fakeSegmentDelay. The not-ready
			// wait starts once it exists, so Ready has really read a playlist without segments.
			if mode == fakeModeHLSHeaderOnly {
				awaitFile(t, filepath.Join(test.job.OutputDir, hlsPlaylistName))
			}

			if err := readyWithin(t, production, notReadyWait); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("Ready = %v, want context.DeadlineExceeded", err)
			}

			if isDone(production) {
				t.Error("the process ended although only the caller gave up")
			}
		})
	}
}

func TestProducerStopIsIdempotentAndSafeWithReady(t *testing.T) {
	test := newProducerTest(t, fakeModeHang)
	production := test.start(t.Context(), t)

	var (
		group   sync.WaitGroup
		results = make(chan error, concurrentCallers)
	)

	for range concurrentCallers {
		group.Go(func() { results <- readyWithin(t, production, readyDeadline) })
	}

	for range concurrentCallers {
		group.Go(production.Stop)
	}

	group.Wait()
	close(results)

	for err := range results {
		assertStopped(t, err)
	}

	if !isDone(production) {
		t.Fatal("Done is open after Stop returned")
	}

	started := time.Now()
	production.Stop()

	if elapsed := time.Since(started); elapsed > time.Second {
		t.Errorf("a repeated Stop took %s", elapsed)
	}

	assertNoWarnings(t, test.logs)
}

func TestProducerStopsWithItsContext(t *testing.T) {
	test := newProducerTest(t, fakeModeHang)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	production := test.start(ctx, t)
	cancel()
	awaitDone(t, production)

	assertStopped(t, production.Ready(t.Context()))
	assertNoWarnings(t, test.logs)
}

// TestProducerStopGivesUpAfterTheStopTimeout: a stray child of FFmpeg keeps its error output
// open, so after the kill the process cannot be reaped before processWaitDelay. Done stays
// open however fast the kill is, and only the stop timeout can end Stop: the test does not
// depend on which of two ready select cases Go picks.
func TestProducerStopGivesUpAfterTheStopTimeout(t *testing.T) {
	const testStopTimeout = 50 * time.Millisecond

	holderFile := filepath.Join(t.TempDir(), "holder.pid")
	t.Setenv(fakeHolderVariable, holderFile)

	test := newProducerTest(t, fakeModeHangHoldingStderr)
	test.producer.stopTimeout = testStopTimeout
	production := test.start(t.Context(), t)
	holder := awaitHolder(t, holderFile)

	production.Stop()

	if isDone(production) {
		t.Fatal("Done is closed although the stray child still holds the error output")
	}

	warnings := test.logs.warnings(t)
	if len(warnings) != 1 || warnings[0]["session"] != testSessionID {
		t.Errorf("warnings = %v, want one about the slow exit", warnings)
	}

	// Once the stray child is gone, the process is reaped and reported as stopped.
	_ = holder.Kill()

	awaitDone(t, production)
	assertStopped(t, production.Ready(t.Context()))

	if after := test.logs.warnings(t); len(after) != 1 {
		t.Errorf("warnings after the end = %v, want only the one about the slow exit", after)
	}
}

// awaitHolder waits until the fake published its stray child's process ID and ends that
// child when the test ends.
func awaitHolder(t *testing.T, holderFile string) *os.Process {
	t.Helper()

	var (
		pid     int
		readErr error
	)

	// Windows may still hold the freshly renamed file for a moment (sharing violation).
	deadline := time.Now().Add(readyDeadline)
	for {
		var text []byte

		text, readErr = os.ReadFile(holderFile) //nolint:gosec // test directory
		if readErr == nil {
			pid, readErr = strconv.Atoi(string(text))
		}

		if readErr == nil {
			break
		}

		if time.Now().After(deadline) {
			t.Fatalf("read the holder pid within %s: %v", readyDeadline, readErr)
		}

		time.Sleep(testPollInterval)
	}

	holder, err := os.FindProcess(pid)
	if err != nil {
		t.Fatalf("find holder %d: %v", pid, err)
	}

	t.Cleanup(func() { _ = holder.Kill() })

	return holder
}

func TestProducerStartFailures(t *testing.T) {
	tests := []struct {
		name      string
		linuxOnly bool
		change    func(t *testing.T, test *producerTest) context.Context
		want      error
	}{
		{
			name: "direct play plan",
			change: func(t *testing.T, test *producerTest) context.Context {
				test.job.Plan.Mode = playback.ModeDirectPlay

				return t.Context()
			},
			want: domain.ErrInvalid,
		},
		{
			name: "missing executable",
			change: func(t *testing.T, test *producerTest) context.Context {
				test.producer.ffmpeg.executable = filepath.Join(t.TempDir(), "no-such-ffmpeg")

				return t.Context()
			},
			want: domain.ErrUnavailable,
		},
		{
			name: "ended context",
			change: func(t *testing.T, _ *producerTest) context.Context {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()

				return ctx
			},
			want: context.Canceled,
		},
		{
			name:      "input missing in the library",
			linuxOnly: true,
			change: func(t *testing.T, test *producerTest) context.Context {
				test.job.RelativePath = "missing.mkv"

				return t.Context()
			},
			want: fs.ErrNotExist,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.linuxOnly && runtime.GOOS != "linux" {
				t.Skip("the producer opens the input itself only on Linux")
			}

			setup := newProducerTest(t, fakeModeHLSLive)
			ctx := test.change(t, &setup)

			production, err := setup.producer.Start(ctx, setup.job)
			if !errors.Is(err, test.want) || production != nil {
				t.Fatalf("Start = %v, %v, want nil, %v", production, err, test.want)
			}

			if !strings.Contains(err.Error(), testSessionID) {
				t.Errorf("error %q does not name the session", err)
			}

			if entries, _ := os.ReadDir(setup.job.OutputDir); len(entries) > 0 {
				t.Errorf("a failed start wrote %v", entries)
			}
		})
	}
}

func TestListsSegment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		playlist string
		want     bool
	}{
		{name: "empty", playlist: "", want: false},
		{name: "header only", playlist: fakePlaylistHeader, want: false},
		{name: "blank lines", playlist: fakePlaylistHeader + "\n  \n", want: false},
		{name: "one segment", playlist: fakePlaylistHeader + fakePlaylistSegment, want: true},
		{name: "windows line ends", playlist: "#EXTM3U\r\n#EXTINF:6.0,\r\nseg-00000.m4s\r\n", want: true},
		{name: "finished", playlist: fakePlaylistHeader + fakePlaylistSegment + fakePlaylistEnd, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := listsSegment(strings.NewReader(test.playlist)); got != test.want {
				t.Errorf("listsSegment(%q) = %t, want %t", test.playlist, got, test.want)
			}
		})
	}
}
