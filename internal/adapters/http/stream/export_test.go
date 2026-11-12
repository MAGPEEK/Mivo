package stream

import "time"

// SetReadyTimeout shortens the readiness wait so tests need not wait 20 seconds.
func SetReadyTimeout(handlers *Handlers, timeout time.Duration) {
	handlers.readyTimeout = timeout
}

// SetConversionTimeout shortens the subtitle conversion wait.
func SetConversionTimeout(handlers *Handlers, timeout time.Duration) {
	handlers.conversionTimeout = timeout
}

// ConversionTimeout exposes the subtitle wait for the test that pins it to the contract.
const ConversionTimeout = conversionTimeout

// ReadyTimeout exposes the production wait for the test that pins it to ADR 0024.
const ReadyTimeout = readyTimeout

// MaxPlaylistBytes exposes the playlist read bound for the tests at and beyond it.
const MaxPlaylistBytes = maxPlaylistBytes
