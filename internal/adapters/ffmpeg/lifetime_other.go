//go:build !linux && !windows

package ffmpeg

import (
	"os"
	"os/exec"
)

// childLifetime does nothing on systems without a parent-death signal or job objects
// (macOS, the BSDs): there a tool outlives a server that crashed, until it reaches the end of
// its input. Stop, the idle timeout and a clean shutdown still kill it (ADR 0024 Notes
// M2-24b).
type childLifetime struct{}

func bindChildLifetime(*exec.Cmd) (*childLifetime, error) {
	return &childLifetime{}, nil
}

func (*childLifetime) adopt(*os.Process) error {
	return nil
}

func (*childLifetime) release() {}
