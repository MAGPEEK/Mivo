package ffmpeg

import (
	"os"
	"os/exec"
	"runtime"
	"syscall"
)

// childLifetime makes the kernel kill a tool when the server dies, however it dies: the
// child gets SIGKILL as its parent-death signal (Pdeathsig, ADR 0024 Notes M2-24b).
//
// Linux sends that signal when the OS thread that started the child ends, not the process,
// and the Go runtime ends a thread when a goroutine locked to it (runtime.LockOSThread)
// returns without unlocking. The goroutine that starts a tool therefore stays locked to its
// thread until it has waited for the tool: while the child lives no other goroutine runs on
// that thread, so none can end it, and the starting goroutine unlocks before it can return.
// Locking only around Start would not do: after the unlock any goroutine, including one of a
// dependency, could lock the same thread and end it, killing the child mid-stream.
//
// The cost is one OS thread per running tool, which the Go runtime would block in the wait
// anyway.
type childLifetime struct{}

// bindChildLifetime prepares command and pins the calling goroutine to its thread; release
// must be called on the same goroutine once the child has been waited for or did not start.
func bindChildLifetime(command *exec.Cmd) (*childLifetime, error) {
	runtime.LockOSThread()

	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}

	command.SysProcAttr.Pdeathsig = syscall.SIGKILL

	return &childLifetime{}, nil
}

// adopt has nothing to do on Linux: the parent-death signal is set up by the start itself.
func (*childLifetime) adopt(*os.Process) error {
	return nil
}

// release unpins the goroutine from its thread.
func (*childLifetime) release() {
	runtime.UnlockOSThread()
}
