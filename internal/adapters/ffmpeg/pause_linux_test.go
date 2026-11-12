package ffmpeg

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
)

const (
	// stoppedState is the state letter /proc/<pid>/stat shows for a process stopped by SIGSTOP.
	stoppedState = "T"
	// stateDeadline bounds the wait for the kernel to show a state change.
	stateDeadline = 10 * time.Second
)

// processState returns the state letter of pid from /proc/<pid>/stat; the state follows the
// closing parenthesis of the command name.
func processState(t *testing.T, pid int) string {
	t.Helper()

	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		t.Fatalf("read the process state: %v", err)
	}

	text := string(data)
	fields := strings.Fields(text[strings.LastIndex(text, ")")+1:])

	return fields[0]
}

func awaitProcessState(t *testing.T, pid int, stopped bool) {
	t.Helper()

	deadline := time.Now().Add(stateDeadline)
	for time.Now().Before(deadline) {
		if (processState(t, pid) == stoppedState) == stopped {
			return
		}

		time.Sleep(testPollInterval)
	}

	t.Fatalf("process %d did not reach stopped=%v within %s", pid, stopped, stateDeadline)
}

func TestProducerPausesAndResumesTheProcess(t *testing.T) {
	test := newProducerTest(t, fakeModeHLSLive)
	running := test.start(t.Context(), t)

	pausable, ok := running.(appplayback.Pausable)
	if !ok || !pausable.Pausable() {
		t.Fatal("the production is not pausable on Linux")
	}

	started, ok := running.(*production)
	if !ok {
		t.Fatalf("the producer returned %T", running)
	}

	pid := started.pause.process.Pid

	if err := pausable.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	awaitProcessState(t, pid, true)

	if err := pausable.Pause(); err != nil {
		t.Fatalf("second Pause: %v", err)
	}

	if err := pausable.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	awaitProcessState(t, pid, false)

	if err := readyWithin(t, running, readyDeadline); err != nil {
		t.Fatalf("Ready after Resume: %v", err)
	}
}

func TestProducerStopKillsAPausedProcess(t *testing.T) {
	test := newProducerTest(t, fakeModeHLSLive)
	running := test.start(t.Context(), t)

	pausable, ok := running.(appplayback.Pausable)
	if !ok {
		t.Fatal("the production does not implement Pausable")
	}

	if err := pausable.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	running.Stop()

	if !isDone(running) {
		t.Fatal("Done is open after Stop of a paused production")
	}

	assertStopped(t, running.Ready(t.Context()))
	assertNoWarnings(t, test.logs)
}
