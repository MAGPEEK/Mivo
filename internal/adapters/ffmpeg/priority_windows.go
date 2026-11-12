//go:build windows

package ffmpeg

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// prepareLowPriority starts the tool in the below-normal priority class, so the class applies
// from the first instruction and to every thread.
func prepareLowPriority(command *exec.Cmd) {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}

	command.SysProcAttr.CreationFlags |= windows.BELOW_NORMAL_PRIORITY_CLASS
}

// lowerPriority has nothing to do after the start on Windows.
func lowerPriority(*os.Process) error {
	return nil
}
