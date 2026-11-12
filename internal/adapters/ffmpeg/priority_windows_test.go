package ffmpeg

import (
	"testing"

	"golang.org/x/sys/windows"
)

// priorityClass reads the priority class of a process.
func priorityClass(t *testing.T, pid int) uint32 {
	t.Helper()

	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid)) //nolint:gosec // process IDs fit in a DWORD
	if err != nil {
		t.Fatalf("open process: %v", err)
	}

	defer func() { _ = windows.CloseHandle(handle) }()

	class, err := windows.GetPriorityClass(handle)
	if err != nil {
		t.Fatalf("get priority class: %v", err)
	}

	return class
}

func TestLowPriorityToolRunsBelowNormal(t *testing.T) {
	child := startHangingChild(t, processFiles{lowPriority: true})

	if got := priorityClass(t, child.command.Process.Pid); got != windows.BELOW_NORMAL_PRIORITY_CLASS {
		t.Errorf("priority class = %#x, want BELOW_NORMAL_PRIORITY_CLASS", got)
	}
}

func TestNormalPriorityToolKeepsTheNormalClass(t *testing.T) {
	child := startHangingChild(t, processFiles{})

	if got := priorityClass(t, child.command.Process.Pid); got != windows.NORMAL_PRIORITY_CLASS {
		t.Errorf("priority class = %#x, want NORMAL_PRIORITY_CLASS", got)
	}
}
