//go:build !linux && !windows

package ffmpeg

import (
	"os"
	"os/exec"
)

// prepareLowPriority does nothing on systems without a supported priority mechanism.
func prepareLowPriority(*exec.Cmd) {}

// lowerPriority does nothing on systems without a supported priority mechanism.
func lowerPriority(*os.Process) error {
	return nil
}
