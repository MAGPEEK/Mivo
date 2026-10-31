package ffmpeg

import (
	"golang.org/x/sys/windows"
)

// processGone reports whether the process has ended. OpenProcess fails for an ID that names
// no process; a handle to an ended process is signalled.
func processGone(pid int) bool {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid)) //nolint:gosec // process IDs fit in a DWORD
	if err != nil {
		return true
	}

	defer func() { _ = windows.CloseHandle(handle) }()

	event, err := windows.WaitForSingleObject(handle, 0)

	return err == nil && event == windows.WAIT_OBJECT_0
}
