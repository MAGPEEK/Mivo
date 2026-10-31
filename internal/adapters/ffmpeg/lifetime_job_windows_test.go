package ffmpeg

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	// jobStressBindsVariable raises the number of binds of
	// TestBindChildLifetimeUnderGCPressure, for example to 5000000 after touching the job
	// object code.
	jobStressBindsVariable = "MIVO_FFMPEG_JOB_STRESS_BINDS"
	// defaultJobStressBinds keeps the test at about a second while the old stack bug (about
	// 6 in a million binds) would still show up in most runs.
	defaultJobStressBinds = 500000
	// jobStressWorkers bind job objects concurrently, each at changing stack depths.
	jobStressWorkers = 16
	// jobStressMaxDepth spreads the binds over many stack depths, so some of them cross the
	// boundary where the goroutine's stack has to grow during the call.
	jobStressMaxDepth = 32
	// jobStressAllocators churn the heap so the collector runs and shrinks stacks.
	jobStressAllocators = 4
	// jobStressAllocation is the size of each garbage allocation.
	jobStressAllocation = 4 << 10
)

// TestBindChildLifetimeSetsKillOnJobClose: the job object must carry
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE, otherwise a crashed server leaves its tools running.
func TestBindChildLifetimeSetsKillOnJobClose(t *testing.T) {
	t.Parallel()

	lifetime, err := bindChildLifetime(&exec.Cmd{})
	if err != nil {
		t.Fatalf("bindChildLifetime: %v", err)
	}

	defer lifetime.release()

	if err := assertKillOnJobClose(lifetime.job); err != nil {
		t.Error(err)
	}
}

// TestBindChildLifetimeUnderGCPressure binds many job objects at changing stack depths while
// other goroutines keep the collector busy. The limits must reach Windows intact every time:
// a Go value passed by address to a system call wrapper that is not protected by the
// compiler may move with its stack during the call (M2-24b test report F2).
func TestBindChildLifetimeUnderGCPressure(t *testing.T) {
	binds := jobStressBinds(t)

	var stop atomic.Bool
	churn := churnGarbage(&stop)

	defer func() {
		stop.Store(true)
		churn.Wait()
	}()

	var (
		group    sync.WaitGroup
		next     atomic.Int64
		failures atomic.Int64
		first    sync.Once
	)

	for range jobStressWorkers {
		group.Go(func() {
			for index := next.Add(1); index <= int64(binds); index = next.Add(1) {
				atStackDepth(int(index%jobStressMaxDepth), func() {
					if err := bindAndCheck(); err != nil {
						failures.Add(1)
						first.Do(func() { t.Errorf("first failed bind: %v", err) })
					}
				})
			}
		})
	}

	group.Wait()

	if count := failures.Load(); count > 0 {
		t.Errorf("%d of %d binds failed", count, binds)
	}
}

func bindAndCheck() error {
	lifetime, err := bindChildLifetime(&exec.Cmd{})
	if err != nil {
		return err
	}

	defer lifetime.release()

	return assertKillOnJobClose(lifetime.job)
}

// churnGarbage runs a collector loop and allocating goroutines until stop is set.
func churnGarbage(stop *atomic.Bool) *sync.WaitGroup {
	var group sync.WaitGroup

	group.Go(func() {
		for !stop.Load() {
			runtime.GC()
		}
	})

	for range jobStressAllocators {
		group.Go(func() {
			var keep [][]byte

			for !stop.Load() {
				keep = append(keep, make([]byte, jobStressAllocation))
				if len(keep) > jobStressMaxDepth {
					keep = keep[:0]
				}
			}
		})
	}

	return &group
}

// atStackDepth runs action below depth extra frames of padding.
//
//go:noinline
func atStackDepth(depth int, action func()) {
	var padding [256]byte

	if depth <= 0 {
		action()

		return
	}

	padding[depth%len(padding)] = 1
	atStackDepth(depth-1, action)
	runtime.KeepAlive(&padding)
}

func jobStressBinds(t *testing.T) int {
	t.Helper()

	text := os.Getenv(jobStressBindsVariable)
	if text == "" {
		return defaultJobStressBinds
	}

	binds, err := strconv.Atoi(text)
	if err != nil || binds <= 0 {
		t.Fatalf("%s=%q is not a positive number", jobStressBindsVariable, text)
	}

	return binds
}

// assertKillOnJobClose reads the job's limits back from Windows.
func assertKillOnJobClose(job windows.Handle) error {
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION

	if err := queryJobInformation(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil { //nolint:gosec // rule 4, see queryJobInformation
		return fmt.Errorf("query job limits: %w", err)
	}

	if flags := limits.BasicLimitInformation.LimitFlags; flags&windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE == 0 {
		return fmt.Errorf("job limit flags = %#x, want JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE", flags)
	}

	return nil
}

// queryJobInformation is QueryInformationJobObject for a Go value whose address is converted
// in the call expression; see setJobInformation for the directive.
//
//go:uintptrescapes
func queryJobInformation(job windows.Handle, class int32, information uintptr, length uint32) error {
	if err := windows.QueryInformationJobObject(job, class, information, length, nil); err != nil {
		return fmt.Errorf("query job object: %w", err)
	}

	return nil
}
