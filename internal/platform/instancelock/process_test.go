package instancelock_test

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/platform/brand"
	"github.com/magpeek/mivo/internal/platform/instancelock"
)

// The test binary doubles as a second server process: with helperModeVariable set, TestMain
// takes the lock (or tries to) on the directory in helperDirVariable instead of running the
// tests. Only a real second process proves that the OS lock works across processes and is
// released when the holder dies.
const (
	helperModeVariable = brand.EnvPrefix + "_INSTANCELOCK_HELPER_MODE"
	helperDirVariable  = brand.EnvPrefix + "_INSTANCELOCK_HELPER_DIR"

	// helperModeHold acquires the lock, reports helperLocked and holds the lock until its
	// standard input is closed or the process is killed.
	helperModeHold = "hold"
	// helperModeTry acquires and releases the lock once and reports the outcome.
	helperModeTry = "try"

	helperLocked   = "locked"
	helperAcquired = "acquired"
	helperRefused  = "refused: "
	helperFailed   = "failed: "

	helperExitFailure = 1

	// helperTimeout bounds every wait for the helper process; starting a race-enabled test
	// binary takes a moment on slow machines.
	helperTimeout = 30 * time.Second

	// releaseTimeout bounds the wait for the OS to drop the lock of a killed process.
	// Windows documents that this "depends upon available system resources".
	releaseTimeout = 10 * time.Second
	releasePoll    = 20 * time.Millisecond
)

func TestMain(m *testing.M) {
	if mode := os.Getenv(helperModeVariable); mode != "" {
		os.Exit(runHelper(mode, os.Getenv(helperDirVariable)))
	}

	os.Exit(m.Run())
}

func runHelper(mode, dataDir string) int {
	lock, err := instancelock.Acquire(dataDir)

	switch {
	case errors.Is(err, instancelock.ErrLocked):
		fmt.Println(helperRefused + err.Error())

		return 0
	case err != nil:
		fmt.Println(helperFailed + err.Error())

		return helperExitFailure
	}

	if mode == helperModeHold {
		fmt.Println(helperLocked)
		// Blocks until the parent closes the pipe; the parent may kill the process instead.
		_, _ = io.Copy(io.Discard, os.Stdin)
	} else {
		fmt.Println(helperAcquired)
	}

	if err := lock.Release(); err != nil {
		fmt.Println(helperFailed + err.Error())

		return helperExitFailure
	}

	return 0
}

func TestAcquireFailsWhileAnotherProcessHoldsTheLock(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	lock := acquire(t, dataDir)

	output := runTryHelper(t, dataDir)

	message, refused := strings.CutPrefix(output, helperRefused)
	if !refused {
		t.Fatalf("helper reported %q, want %q prefix", output, helperRefused)
	}

	if !strings.Contains(message, dataDir) {
		t.Errorf("helper error %q does not name the data directory %s", message, dataDir)
	}

	releaseOrFail(t, lock)

	if output := runTryHelper(t, dataDir); output != helperAcquired {
		t.Errorf("helper after Release reported %q, want %q", output, helperAcquired)
	}
}

func TestLockOfAKilledProcessIsReleased(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	holder := startHolder(t, dataDir)

	locked, err := instancelock.Acquire(dataDir)
	if err == nil {
		releaseOrFail(t, locked)
		t.Fatal("Acquire succeeded while another process holds the lock")
	}

	assertLockedError(t, err, dataDir)

	if err := holder.Process.Kill(); err != nil {
		t.Fatalf("kill holder: %v", err)
	}

	// A killed process exits with an error status; only the exit matters here.
	_ = holder.Wait()

	releaseOrFail(t, acquireEventually(t, dataDir))
}

func TestAcquireSucceedsAfterTheOtherProcessReleased(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	holder := startHolder(t, dataDir)

	if err := holder.stdin.Close(); err != nil {
		t.Fatalf("close holder stdin: %v", err)
	}

	if err := holder.Wait(); err != nil {
		t.Fatalf("holder exited with %v", err)
	}

	releaseOrFail(t, acquire(t, dataDir))
}

// holderProcess is a helper process that holds the lock until stdin is closed.
type holderProcess struct {
	*exec.Cmd
	stdin io.Closer
}

// startHolder starts a helper process that holds the lock on dataDir and returns once it
// reported the lock. Cleanup kills it, so a failing test never leaves it running.
func startHolder(t *testing.T, dataDir string) holderProcess {
	t.Helper()

	command := helperCommand(t, helperModeHold, dataDir)

	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatalf("holder stdin: %v", err)
	}

	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("holder stdout: %v", err)
	}

	if err := command.Start(); err != nil {
		t.Fatalf("start holder: %v", err)
	}

	t.Cleanup(func() {
		_ = stdin.Close()
		_ = command.Process.Kill()
		_ = command.Wait()
	})

	if line := readLine(t, stdout); line != helperLocked {
		t.Fatalf("holder reported %q, want %q", line, helperLocked)
	}

	return holderProcess{Cmd: command, stdin: stdin}
}

// runTryHelper runs a helper process that tries to take the lock once and returns its
// one-line report.
func runTryHelper(t *testing.T, dataDir string) string {
	t.Helper()

	output, err := helperCommand(t, helperModeTry, dataDir).Output()
	if err != nil {
		t.Fatalf("try helper: %v (output %q)", err, output)
	}

	return strings.TrimSpace(string(output))
}

func helperCommand(t *testing.T, mode, dataDir string) *exec.Cmd {
	t.Helper()

	// The test binary re-executes itself; TestMain turns it into the helper.
	command := exec.CommandContext(t.Context(), os.Args[0]) //nolint:gosec // G204: the test binary itself
	command.Env = append(os.Environ(), helperModeVariable+"="+mode, helperDirVariable+"="+dataDir)
	command.Stderr = os.Stderr
	command.WaitDelay = helperTimeout

	return command
}

// readLine reads the first line of a helper's output within helperTimeout.
func readLine(t *testing.T, output io.Reader) string {
	t.Helper()

	lines := make(chan string, 1)

	go func() {
		line, _ := bufio.NewReader(output).ReadString('\n')
		lines <- strings.TrimSpace(line)
	}()

	select {
	case line := <-lines:
		return line
	case <-time.After(helperTimeout):
		t.Fatalf("helper did not report within %s", helperTimeout)
	}

	return ""
}

// acquireEventually retries Acquire until the OS has dropped the lock of a dead process.
func acquireEventually(t *testing.T, dataDir string) *instancelock.Lock {
	t.Helper()

	deadline := time.Now().Add(releaseTimeout)

	for {
		lock, err := instancelock.Acquire(dataDir)
		if err == nil {
			return lock
		}

		if !errors.Is(err, instancelock.ErrLocked) || time.Now().After(deadline) {
			t.Fatalf("Acquire after the holder died: %v", err)
		}

		time.Sleep(releasePoll)
	}
}
