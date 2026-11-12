//go:build !linux

package ffmpeg

import (
	"errors"
	"testing"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
)

func TestProducerOutsideLinuxIsNotPausable(t *testing.T) {
	test := newProducerTest(t, fakeModeHLSLive)
	production := test.start(t.Context(), t)

	pausable, ok := production.(appplayback.Pausable)
	if !ok {
		t.Fatal("the production does not implement Pausable")
	}

	if pausable.Pausable() {
		t.Error("Pausable() = true on a system without job control signals")
	}

	if err := pausable.Pause(); !errors.Is(err, ErrPauseUnsupported) {
		t.Errorf("Pause = %v, want ErrPauseUnsupported", err)
	}
}
