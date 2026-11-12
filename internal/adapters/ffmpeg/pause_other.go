//go:build !linux

package ffmpeg

import "os"

// pauseSupported is false where the system has no job control signals (Windows, macOS dev
// machines): productions there report Pausable() false.
const pauseSupported = false

// signalProcess is never called while pauseSupported is false.
func signalProcess(*os.Process, bool) error {
	return ErrPauseUnsupported
}
