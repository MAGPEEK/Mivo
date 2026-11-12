package ffmpeg

import (
	"errors"
	"os"
	"testing"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain"
)

// signalRecorder is a signalProcess that records the calls instead of sending signals.
type signalRecorder struct {
	calls []bool
	err   error
}

func (r *signalRecorder) signal(_ *os.Process, stop bool) error {
	r.calls = append(r.calls, stop)

	return r.err
}

// newPauseTestProduction returns a production with a recording signal and a bound process.
func newPauseTestProduction(recorder *signalRecorder) *production {
	p := &production{sessionID: testSessionID, done: make(chan struct{})}
	p.pause.signal = recorder.signal
	p.pause.bind(&os.Process{})

	return p
}

func TestPauseAndResumeAreIdempotent(t *testing.T) {
	recorder := &signalRecorder{}
	p := newPauseTestProduction(recorder)

	steps := []struct {
		name string
		run  func() error
		want []bool
	}{
		{"resume of a running production", p.Resume, nil},
		{"pause", p.Pause, []bool{true}},
		{"second pause", p.Pause, []bool{true}},
		{"resume", p.Resume, []bool{true, false}},
		{"second resume", p.Resume, []bool{true, false}},
	}

	for _, step := range steps {
		if err := step.run(); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}

		if len(recorder.calls) != len(step.want) {
			t.Fatalf("%s: signals = %v, want %v", step.name, recorder.calls, step.want)
		}
	}

	if !p.Pausable() {
		t.Error("Pausable() = false with a signal function")
	}
}

func TestStopResumesAPausedProduction(t *testing.T) {
	recorder := &signalRecorder{}
	p := newPauseTestProduction(recorder)
	p.cancel = func() {}
	close(p.done)

	p.pause.paused = true
	p.Stop()

	if len(recorder.calls) != 1 || recorder.calls[0] {
		t.Errorf("signals = %v, want one continue", recorder.calls)
	}
}

func TestPauseFailsAfterStop(t *testing.T) {
	recorder := &signalRecorder{}
	p := newPauseTestProduction(recorder)
	close(p.done)

	for name, run := range map[string]func() error{"Pause": p.Pause, "Resume": p.Resume} {
		if err := run(); !errors.Is(err, appplayback.ErrProductionStopped) {
			t.Errorf("%s after the end = %v, want ErrProductionStopped", name, err)
		}
	}

	if len(recorder.calls) != 0 {
		t.Errorf("signals = %v, want none", recorder.calls)
	}
}

func TestPauseKeepsTheStateWhenTheSignalFails(t *testing.T) {
	signalFailure := errors.New("no such process")
	recorder := &signalRecorder{err: signalFailure}
	p := newPauseTestProduction(recorder)

	if err := p.Pause(); !errors.Is(err, signalFailure) {
		t.Fatalf("Pause = %v, want the signal error", err)
	}

	recorder.err = nil

	if err := p.Pause(); err != nil || len(recorder.calls) != 2 {
		t.Errorf("a retried Pause = %v after %d signals, want nil after 2", err, len(recorder.calls))
	}
}

func TestProductionWithoutSignalIsNotPausable(t *testing.T) {
	p := &production{sessionID: testSessionID, done: make(chan struct{})}

	if p.Pausable() {
		t.Error("Pausable() = true without a signal function")
	}

	for name, run := range map[string]func() error{"Pause": p.Pause, "Resume": p.Resume} {
		err := run()
		if !errors.Is(err, ErrPauseUnsupported) || !errors.Is(err, domain.ErrUnavailable) {
			t.Errorf("%s = %v, want ErrPauseUnsupported wrapping ErrUnavailable", name, err)
		}
	}
}
