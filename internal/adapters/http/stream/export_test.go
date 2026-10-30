package stream

import "time"

// SetReadyTimeout shortens the readiness wait so tests need not wait 20 seconds.
func SetReadyTimeout(handlers *Handlers, timeout time.Duration) {
	handlers.readyTimeout = timeout
}

// ReadyTimeout exposes the production wait for the test that pins it to ADR 0024.
const ReadyTimeout = readyTimeout
