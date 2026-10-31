//go:build linux || windows

package ffmpeg

import (
	"bufio"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// processPollInterval is how often the lifetime test looks for the orphan.
const processPollInterval = 20 * time.Millisecond

// TestChildDiesWithItsParent kills a stand-in server without any cleanup, the way a crash or
// SIGKILL ends Mivo, and checks that the FFmpeg stand-in it started is gone as well: Linux
// delivers the parent-death signal, Windows closes the job object (ADR 0024 Notes M2-24b).
func TestChildDiesWithItsParent(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("test executable: %v", err)
	}

	t.Setenv(fakeModeVariable, fakeModeSpawnHang)

	// The executable is this test binary; no shell is involved.
	parent := exec.CommandContext(t.Context(), executable) //nolint:gosec // see above
	parent.Stderr = os.Stderr

	stdout, err := parent.StdoutPipe()
	if err != nil {
		t.Fatalf("parent stdout: %v", err)
	}

	if err := parent.Start(); err != nil {
		t.Fatalf("start the parent: %v", err)
	}

	childPID := readChildPID(t, bufio.NewScanner(stdout))
	t.Cleanup(func() { killStray(childPID) })

	if processGone(childPID) {
		t.Fatalf("the child %d is not running before its parent is killed", childPID)
	}

	if err := parent.Process.Kill(); err != nil {
		t.Fatalf("kill the parent: %v", err)
	}

	// The parent was killed, so Wait reports it; only the reaping matters.
	_ = parent.Wait()

	deadline := time.Now().Add(killDeadline)
	for !processGone(childPID) {
		if time.Now().After(deadline) {
			t.Fatalf("the child %d still runs %s after its parent was killed", childPID, killDeadline)
		}

		time.Sleep(processPollInterval)
	}
}

func readChildPID(t *testing.T, lines *bufio.Scanner) int {
	t.Helper()

	if !lines.Scan() {
		t.Fatalf("the parent printed no child process ID: %v", lines.Err())
	}

	pid, err := strconv.Atoi(strings.TrimSpace(lines.Text()))
	if err != nil {
		t.Fatalf("child process ID %q: %v", lines.Text(), err)
	}

	return pid
}

// killStray ends a child the lifetime guard failed to end, so a failing test leaves no
// process behind. An error means it is gone already.
func killStray(pid int) {
	if processGone(pid) {
		return
	}

	if process, err := os.FindProcess(pid); err == nil {
		_ = process.Kill()
	}
}
