package ffmpeg

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	// exitGraceMilliseconds bounds how long adopt waits for a process the job refused because
	// it is already exiting: its exit code is set when termination begins, but its handle is
	// signalled only once termination has finished.
	exitGraceMilliseconds uint32 = 100
	// stillActive is the exit code Windows reports for a process that is not terminating
	// (STILL_ACTIVE).
	stillActive = 259
)

// childLifetime makes Windows kill a tool when the server dies, however it dies: every tool
// runs in its own job object with JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE, and the server holds
// the only handle to it. When the server exits or crashes, Windows closes the handle and
// ends every process in the job (ADR 0024 Notes M2-24b). Processes FFmpeg starts itself join
// the job too.
//
// The tool is assigned right after the start, so it runs for a moment outside its job: a
// server killed in exactly that moment leaves it running. Starting it suspended would close
// that window, but os/exec closes the handle of the primary thread, and finding the thread
// again with the documented Toolhelp snapshot costs 100–150 ms per start on a desktop with
// thousands of threads (measured), far more than a quick ffprobe run. A tool that exits
// within that moment cannot be assigned any more (Windows answers ACCESS_DENIED once it is
// terminating) and needs no guard.
//
// One job per tool needs no shared state: release closes the handle after the tool has been
// waited for.
type childLifetime struct {
	job windows.Handle
}

// bindChildLifetime creates the job object for command's process.
func bindChildLifetime(*exec.Cmd) (*childLifetime, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create job object: %w", err)
	}

	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}

	// The conversion must stay inside the call expression; see setJobInformation.
	err = setJobInformation(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))) //nolint:gosec // rule 4, see setJobInformation
	if err != nil {
		closeHandle(job)

		return nil, fmt.Errorf("configure job object: %w", err)
	}

	return &childLifetime{job: job}, nil
}

// setJobInformation hands SetInformationJobObject a Go value by address, the uintptr
// information, converted from an unsafe.Pointer in the call expression. x/sys/windows takes
// that address as a plain uintptr and passes it on through a Go function, a case the
// compiler does not protect: the value stayed on the goroutine's stack, a stack growth or
// shrink during the call moved it, and Windows read freed memory – a spurious "parameter is
// incorrect" or, worse, a job without JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE (M2-24b test report
// F2, about 6 in a million binds under GC pressure). The directive makes the compiler move
// every value converted in the argument list to the heap and keep it alive until the call
// returns (unsafe.Pointer rule 4).
//
//go:uintptrescapes
func setJobInformation(job windows.Handle, class uint32, information uintptr, length uint32) error {
	if _, err := windows.SetInformationJobObject(job, class, information, length); err != nil {
		return fmt.Errorf("set job information: %w", err)
	}

	return nil
}

// adopt assigns the started process to the job. os.Process keeps a handle to the process
// open, so its ID cannot be reused before the process has been waited for.
func (l *childLifetime) adopt(process *os.Process) error {
	processID := uint32(process.Pid) //nolint:gosec // process IDs are DWORDs
	access := uint32(windows.PROCESS_SET_QUOTA | windows.PROCESS_TERMINATE |
		windows.PROCESS_QUERY_LIMITED_INFORMATION | windows.SYNCHRONIZE)

	handle, err := windows.OpenProcess(access, false, processID)
	if err != nil {
		return fmt.Errorf("open process %d: %w", processID, err)
	}

	defer closeHandle(handle)

	err = windows.AssignProcessToJobObject(l.job, handle)
	if err == nil || (errors.Is(err, windows.ERROR_ACCESS_DENIED) && isTerminating(handle)) {
		return nil
	}

	return fmt.Errorf("assign process %d to its job object: %w", processID, err)
}

// isTerminating reports whether the process has begun to exit: its exit code is set, or its
// handle is signalled within exitGraceMilliseconds.
func isTerminating(process windows.Handle) bool {
	var exitCode uint32
	if err := windows.GetExitCodeProcess(process, &exitCode); err == nil && exitCode != stillActive {
		return true
	}

	event, err := windows.WaitForSingleObject(process, exitGraceMilliseconds)

	return err == nil && event == windows.WAIT_OBJECT_0
}

// release closes the job. Called while the tool still ran, it would kill it.
func (l *childLifetime) release() {
	closeHandle(l.job)
}

// closeHandle closes a handle the server opened; a failure leaves nothing to recover.
func closeHandle(handle windows.Handle) {
	_ = windows.CloseHandle(handle)
}
