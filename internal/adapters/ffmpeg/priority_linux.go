//go:build linux

package ffmpeg

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// lowPriorityNiceness is the niceness of a low-priority tool: well below the web app and the
// scanner, but not the 19 of an idle job, which a busy NAS would starve completely.
const lowPriorityNiceness = 10

// prepareLowPriority has nothing to set before the start on Linux: SysProcAttr offers no
// niceness.
func prepareLowPriority(*exec.Cmd) {}

// lowerPriority sets the niceness of the started process. FFmpeg's threads created from now
// on inherit it. The process runs at normal priority for the moment between its start and this
// call, and a thread it created within that moment keeps the normal priority; both are
// negligible next to a transcode of minutes.
func lowerPriority(process *os.Process) error {
	if err := syscall.Setpriority(syscall.PRIO_PROCESS, process.Pid, lowPriorityNiceness); err != nil {
		return fmt.Errorf("set niceness of process %d: %w", process.Pid, err)
	}

	return nil
}
