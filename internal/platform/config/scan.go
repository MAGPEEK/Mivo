package config

import (
	"fmt"
	"strings"
	"time"
)

// Bounds of scan.interval (ADR 0027 §9). Below 15 minutes a large library on a NAS would
// be rescanned almost continuously; above a week the periodic scan no longer catches
// changes the watcher missed in useful time.
const (
	minScanInterval = 15 * time.Minute
	maxScanInterval = 168 * time.Hour
	// disabledScanInterval turns the periodic scan off.
	disabledScanInterval time.Duration = 0
)

// applyScanWatch accepts only true and false, like server.lan_only.
func applyScanWatch(cfg *Config, raw, _ string) error {
	value, err := parseStrictBool(raw)
	if err != nil {
		return err
	}

	cfg.Scan.Watch = value

	return nil
}

// applyScanInterval accepts a Go duration ("12h", "90m"): 0 turns the periodic scan off,
// anything else must lie between the bounds. A bare number is rejected because its unit
// would be a guess.
func applyScanInterval(cfg *Config, raw, _ string) error {
	interval, err := time.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("%w: scan interval %q is not a duration such as 12h or 90m", ErrInvalid, raw)
	}

	if interval != disabledScanInterval && (interval < minScanInterval || interval > maxScanInterval) {
		return fmt.Errorf("%w: scan interval %s is outside %s-%s; use 0 to turn the periodic scan off",
			ErrInvalid, formatDuration(interval), formatDuration(minScanInterval), formatDuration(maxScanInterval))
	}

	cfg.Scan.Interval = interval

	return nil
}

// formatDuration drops the zero minutes and seconds time.Duration.String adds, so messages
// show "15m" and "168h" like the documentation instead of "15m0s" and "168h0m0s".
func formatDuration(duration time.Duration) string {
	text := duration.String()
	if strings.HasSuffix(text, "m0s") {
		text = strings.TrimSuffix(text, "0s")
	}

	if strings.HasSuffix(text, "h0m") {
		text = strings.TrimSuffix(text, "0m")
	}

	return text
}
