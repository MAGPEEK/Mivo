package ffmpeg

import (
	"fmt"
	"os"
	"syscall"
)

// pauseSupported tells that the system can freeze a process. Only Linux, the platform of the
// NAS targets, offers the job control signals used here.
const pauseSupported = true

// signalProcess freezes (stop) or continues a process. os.Process.Signal is safe from any
// goroutine, so the thread pinning of the process start (lifetime_linux.go) does not apply.
func signalProcess(process *os.Process, stop bool) error {
	signal := syscall.SIGCONT
	if stop {
		signal = syscall.SIGSTOP
	}

	if err := process.Signal(signal); err != nil {
		return fmt.Errorf("send %s: %w", signal, err)
	}

	return nil
}
