package ffmpeg

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestAdoptAcceptsAProcessThatAlreadyEnded: a quick ffprobe run can end before it is
// assigned to its job object, which Windows then refuses. Such a process needs no guard, so
// the start must not fail. (A process caught while it terminates takes the same path; the
// stress test TestManyShortLivedStartsSucceed hits that state.)
func TestAdoptAcceptsAProcessThatAlreadyEnded(t *testing.T) {
	command := startEndingFake(t)

	lifetime, err := bindChildLifetime(command)
	if err != nil {
		t.Fatalf("bindChildLifetime: %v", err)
	}

	defer lifetime.release()

	if err := lifetime.adopt(command.Process); err != nil {
		t.Errorf("adopt of an ended process = %v, want nil", err)
	}
}

// startEndingFake starts the fake in a mode that exits at once and returns once it has
// ended, without waiting for it: os.Process keeps the ended process's handle and ID.
func startEndingFake(t *testing.T) *exec.Cmd {
	t.Helper()

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("test executable: %v", err)
	}

	t.Setenv(fakeModeVariable, fakeModeRespond)

	// The executable is this test binary; no shell is involved.
	command := exec.CommandContext(t.Context(), executable) //nolint:gosec // see above
	if err := command.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	t.Cleanup(func() { _ = command.Wait() })

	deadline := time.Now().Add(killDeadline)
	for !processGone(command.Process.Pid) {
		if time.Now().After(deadline) {
			t.Fatalf("the fake did not end within %s", killDeadline)
		}

		time.Sleep(processPollInterval)
	}

	return command
}
