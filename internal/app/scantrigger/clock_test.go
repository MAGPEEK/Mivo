package scantrigger_test

import (
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/scantrigger"
)

func TestSystemClockTicks(t *testing.T) {
	t.Parallel()

	clock := scantrigger.SystemClock{}
	before := time.Now()

	if now := clock.Now(); now.Before(before) {
		t.Errorf("Now = %s, want not before %s", now, before)
	}

	ticker := clock.NewTicker(time.Millisecond)
	defer ticker.Stop()

	select {
	case <-ticker.Ticks():
	case <-time.After(waitTimeout):
		t.Fatal("the system ticker did not tick")
	}
}
