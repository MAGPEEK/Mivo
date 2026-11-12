package ffmpeg

import (
	"fmt"
	"os"
	"runtime"
	"sync"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain"
)

// ErrPauseUnsupported reports Pause or Resume on a system that cannot freeze a process. It
// wraps domain.ErrUnavailable.
var ErrPauseUnsupported = fmt.Errorf("pausing a production is not supported on %s: %w",
	runtime.GOOS, domain.ErrUnavailable)

// PauseSupported reports that productions of this build can be paused (Linux only), for the
// transcoder status the settings screen shows.
func PauseSupported() bool {
	return pauseSupported
}

// pauseControl freezes and continues the process of a production (ADR 0030 §2). signal is
// nil where the system cannot do it.
type pauseControl struct {
	mutex   sync.Mutex
	process *os.Process
	signal  func(process *os.Process, stop bool) error
	paused  bool
}

func (c *pauseControl) supported() bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.signal != nil
}

// bind sets the process to signal once it started. A new process runs, even when the one it
// replaces (the software retry of a failed hardware run) was frozen.
func (c *pauseControl) bind(process *os.Process) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.process = process
	c.paused = false
}

// set freezes (paused true) or continues the process; repeating the current state does
// nothing.
func (c *pauseControl) set(paused bool) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.signal == nil {
		return ErrPauseUnsupported
	}

	if c.process == nil || c.paused == paused {
		return nil
	}

	if err := c.signal(c.process, paused); err != nil {
		return err
	}

	c.paused = paused

	return nil
}

// resumeQuietly continues a frozen process before it is killed. A process that already ended
// cannot be continued, which is fine: the caller kills it anyway.
func (c *pauseControl) resumeQuietly() {
	_ = c.set(false)
}

// Pausable implements appplayback.Pausable.
func (p *production) Pausable() bool {
	return p.pause.supported()
}

// Pause implements appplayback.Pausable: it freezes FFmpeg without killing it. The segments
// already written stay playable; the process continues with Resume. Pausing a paused
// production does nothing. Errors wrap ErrPauseUnsupported or appplayback.ErrProductionStopped
// for a production that ended.
func (p *production) Pause() error {
	return p.setPaused(true)
}

// Resume implements appplayback.Pausable: it continues a paused production; resuming a running
// one does nothing.
func (p *production) Resume() error {
	return p.setPaused(false)
}

func (p *production) setPaused(paused bool) error {
	if !p.pause.supported() {
		return fmt.Errorf("session %s: %w", p.sessionID, ErrPauseUnsupported)
	}

	if p.stopped.Load() || isClosed(p.done) {
		return p.stoppedError()
	}

	if err := p.pause.set(paused); err != nil {
		return fmt.Errorf("session %s: change the pause state: %w", p.sessionID, err)
	}

	return nil
}

func isClosed(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}

var _ appplayback.Pausable = (*production)(nil)
