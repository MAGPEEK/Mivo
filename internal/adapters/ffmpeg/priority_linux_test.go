package ffmpeg

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

// niceness reads the niceness (field 19 of /proc/<pid>/stat) of a process.
func niceness(t *testing.T, pid int) int {
	t.Helper()

	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		t.Fatalf("read stat: %v", err)
	}

	// The fields after the command name (in parentheses) start with the state, field 3.
	_, rest, _ := strings.Cut(string(stat), ") ")
	fields := strings.Fields(rest)

	const nicenessField, firstFieldAfterName = 19, 3

	value, err := strconv.Atoi(fields[nicenessField-firstFieldAfterName])
	if err != nil {
		t.Fatalf("parse niceness: %v", err)
	}

	return value
}

func TestLowPriorityToolRunsWithNiceness10(t *testing.T) {
	child := startHangingChild(t, processFiles{lowPriority: true})

	if child.priorityError != nil {
		t.Fatalf("priority error: %v", child.priorityError)
	}

	if got := niceness(t, child.command.Process.Pid); got != lowPriorityNiceness {
		t.Errorf("niceness = %d, want %d", got, lowPriorityNiceness)
	}
}

func TestNormalPriorityToolKeepsTheServersNiceness(t *testing.T) {
	child := startHangingChild(t, processFiles{})

	if got, want := niceness(t, child.command.Process.Pid), niceness(t, os.Getpid()); got != want {
		t.Errorf("niceness = %d, want the server's %d", got, want)
	}
}
