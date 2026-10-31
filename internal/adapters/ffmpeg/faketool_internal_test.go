package ffmpeg

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The test binary doubles as a fake FFmpeg tool: when fakeModeVariable is set, TestMain
// behaves like the requested tool instead of running tests. This exercises the real process
// handling (pipes, exit codes, kills) on every OS without FFmpeg installed.
const (
	fakeModeVariable = "MIVO_FFMPEG_FAKE_MODE"
	// fakeResponsesVariable names a JSON file that maps a -read_intervals value (or
	// fakeDefaultResponse) to the standard output the fake prints.
	fakeResponsesVariable = "MIVO_FFMPEG_FAKE_RESPONSES"
	// fakeArgumentsVariable names a file to which the fake appends its arguments as one JSON
	// line per call.
	fakeArgumentsVariable = "MIVO_FFMPEG_FAKE_ARGUMENTS"
	fakeDefaultResponse   = "default"

	fakeModeRespond     = "respond"
	fakeModeFail        = "fail"
	fakeModeHang        = "hang"
	fakeModeFloodStdout = "flood-stdout"
	fakeModeFloodStderr = "flood-stderr"
	// fakeModeSpawnHang starts the fake in hang mode as its own child through the runner,
	// prints the child's process ID and hangs: the parent of lifetime tests.
	fakeModeSpawnHang = "spawn-hang"
	// fakeModeHangHoldingStderr starts a hanging grandchild that inherits its standard error,
	// writes the grandchild's process ID to the file named by fakeHolderVariable and hangs:
	// after a kill the error pipe stays open until the test ends the grandchild.
	fakeModeHangHoldingStderr = "hang-holding-stderr"
	fakeHolderVariable        = "MIVO_FFMPEG_FAKE_HOLDER"

	fakeFailExitCode = 3
	fakeFailStderr   = "first line\nInvalid data found when processing input: ü\n"
	fakeHangDuration = 10 * time.Minute
	fakeFloodChunk   = 64 * 1024
)

func TestMain(m *testing.M) {
	if mode := os.Getenv(fakeModeVariable); mode != "" {
		os.Exit(runFakeTool(mode, os.Args[1:]))
	}

	os.Exit(m.Run())
}

func runFakeTool(mode string, arguments []string) int {
	if isFakeHLSMode(mode) {
		return fakeHLS(mode, arguments)
	}

	switch mode {
	case fakeModeRespond:
		return fakeRespond(arguments)
	case fakeModeFail:
		fmt.Fprint(os.Stderr, fakeFailStderr)

		return fakeFailExitCode
	case fakeModeHang:
		time.Sleep(fakeHangDuration)

		return 0
	case fakeModeFloodStdout:
		return fakeFlood(os.Stdout, 0)
	case fakeModeFloodStderr:
		return fakeFlood(os.Stderr, 1)
	case fakeModeSpawnHang:
		return fakeSpawnHang()
	case fakeModeHangHoldingStderr:
		return fakeHangHoldingStderr()
	default:
		fmt.Fprintf(os.Stderr, "unknown fake mode %q\n", mode)

		return 1
	}
}

// fakeSpawnHang plays the server: it starts a hanging child exactly like the producer does,
// reports the child's process ID on standard output and hangs until it is killed.
func fakeSpawnHang() int {
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)

		return 1
	}

	// The child inherits the environment and must hang instead of spawning again.
	if err := os.Setenv(fakeModeVariable, fakeModeHang); err != nil {
		fmt.Fprintln(os.Stderr, err)

		return 1
	}

	child, err := tool{name: "fakechild", executable: executable}.start(context.Background(), nil, processFiles{})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)

		return 1
	}

	if _, err := fmt.Fprintln(os.Stdout, child.command.Process.Pid); err != nil {
		return 1
	}

	time.Sleep(fakeHangDuration)

	return 0
}

// fakeHangHoldingStderr plays an FFmpeg whose stray child keeps its error output open: the
// runner's Wait then cannot finish before processWaitDelay, however fast the kill is.
func fakeHangHoldingStderr() int {
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)

		return 1
	}

	if err := os.Setenv(fakeModeVariable, fakeModeHang); err != nil {
		fmt.Fprintln(os.Stderr, err)

		return 1
	}

	// Started without the runner on purpose: no lifetime guard ties the holder to this
	// process, so it outlives the kill. The test ends it through its process ID.
	holder := exec.Command(executable) //nolint:gosec,noctx // this test binary; ended by the test
	holder.Stderr = os.Stderr

	if err := holder.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)

		return 1
	}

	if err := writeHolderPID(holder.Process.Pid); err != nil {
		fmt.Fprintln(os.Stderr, err)

		return 1
	}

	time.Sleep(fakeHangDuration)

	return 0
}

// writeHolderPID publishes the holder's process ID under a temporary name and renames it,
// so the test never reads a partial file.
func writeHolderPID(pid int) error {
	path := os.Getenv(fakeHolderVariable)

	// The path comes from the test that started the fake, through the environment.
	if err := os.WriteFile(path+fakeTemporarySuffix, []byte(strconv.Itoa(pid)), 0o600); err != nil { //nolint:gosec // see above
		return fmt.Errorf("write holder pid: %w", err)
	}

	if err := os.Rename(path+fakeTemporarySuffix, path); err != nil { //nolint:gosec // see above
		return fmt.Errorf("publish holder pid: %w", err)
	}

	return nil
}

// fakeFlood writes far more than any limit of the runner and stops when the reader is gone.
func fakeFlood(target *os.File, exitCode int) int {
	chunk := []byte(strings.Repeat("é", fakeFloodChunk/2))

	for range (maxOutputBytes / fakeFloodChunk) * 2 {
		if _, err := target.Write(chunk); err != nil {
			return 1
		}
	}

	return exitCode
}

func fakeRespond(arguments []string) int {
	if err := recordFakeArguments(arguments); err != nil {
		fmt.Fprintln(os.Stderr, err)

		return 1
	}

	responses, err := readFakeResponses()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)

		return 1
	}

	key := fakeDefaultResponse
	if position := slices.Index(arguments, "-read_intervals"); position >= 0 && position+1 < len(arguments) {
		key = arguments[position+1]
	}

	response, found := responses[key]
	if !found {
		response = responses[fakeDefaultResponse]
	}

	if _, err := fmt.Fprint(os.Stdout, response); err != nil {
		return 1
	}

	return 0
}

func recordFakeArguments(arguments []string) error {
	path := os.Getenv(fakeArgumentsVariable)
	if path == "" {
		return nil
	}

	line, err := json.Marshal(arguments)
	if err != nil {
		return fmt.Errorf("encode arguments: %w", err)
	}

	// The path comes from the test that started the fake, through the environment.
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // see above
	if err != nil {
		return fmt.Errorf("open arguments file: %w", err)
	}

	_, writeErr := file.Write(append(line, '\n'))
	closeErr := file.Close()

	if writeErr != nil {
		return fmt.Errorf("write arguments: %w", writeErr)
	}

	if closeErr != nil {
		return fmt.Errorf("close arguments file: %w", closeErr)
	}

	return nil
}

func readFakeResponses() (map[string]string, error) {
	path := os.Getenv(fakeResponsesVariable)
	if path == "" {
		return map[string]string{}, nil
	}

	data, err := os.ReadFile(path) //nolint:gosec // the path comes from the test itself
	if err != nil {
		return nil, fmt.Errorf("read responses: %w", err)
	}

	responses := map[string]string{}
	if err := json.Unmarshal(data, &responses); err != nil {
		return nil, fmt.Errorf("decode responses: %w", err)
	}

	return responses, nil
}

// fakeTool configures the fake for one test and returns a tool that starts it. Tests using
// it cannot run in parallel because the configuration travels through the environment.
func fakeTool(t *testing.T, mode string) tool {
	t.Helper()

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("test executable: %v", err)
	}

	t.Setenv(fakeModeVariable, mode)

	return tool{name: "fakeprobe", executable: executable}
}

// fakeResponder makes the fake print responses keyed by -read_intervals value and record
// its arguments. It returns a function that reads the recorded calls.
func fakeResponder(t *testing.T, responses map[string]string) (tool, func() [][]string) {
	t.Helper()

	directory := t.TempDir()
	responsesPath := directory + string(os.PathSeparator) + "responses.json"
	argumentsPath := directory + string(os.PathSeparator) + "arguments.jsonl"

	data, err := json.Marshal(responses)
	if err != nil {
		t.Fatalf("encode responses: %v", err)
	}

	if err := os.WriteFile(responsesPath, data, 0o600); err != nil {
		t.Fatalf("write responses: %v", err)
	}

	t.Setenv(fakeResponsesVariable, responsesPath)
	t.Setenv(fakeArgumentsVariable, argumentsPath)

	return fakeTool(t, fakeModeRespond), func() [][]string { return readRecordedCalls(t, argumentsPath) }
}

func readRecordedCalls(t *testing.T, path string) [][]string {
	t.Helper()

	data, err := os.ReadFile(path) //nolint:gosec // the path comes from the test itself
	if err != nil {
		t.Fatalf("read recorded arguments: %v", err)
	}

	var calls [][]string

	for line := range strings.Lines(string(data)) {
		var arguments []string
		if err := json.Unmarshal([]byte(line), &arguments); err != nil {
			t.Fatalf("decode recorded arguments %q: %v", line, err)
		}

		calls = append(calls, arguments)
	}

	return calls
}

// readFixture returns a file from testdata.
func readFixture(t *testing.T, name string) []byte {
	t.Helper()

	data, err := fs.ReadFile(os.DirFS("testdata"), name)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	return data
}
