package scantrigger

import "time"

// SystemClock is the Clock of the running server: the wall clock and time.Ticker.
type SystemClock struct{}

// Now returns time.Now.
func (SystemClock) Now() time.Time { return time.Now() }

// NewTicker returns a time.Ticker.
func (SystemClock) NewTicker(interval time.Duration) Ticker {
	return systemTicker{ticker: time.NewTicker(interval)}
}

type systemTicker struct {
	ticker *time.Ticker
}

func (adapted systemTicker) Ticks() <-chan time.Time { return adapted.ticker.C }
func (adapted systemTicker) Stop()                   { adapted.ticker.Stop() }
