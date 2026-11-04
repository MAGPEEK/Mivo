package config_test

import (
	"testing"
	"time"
)

// The interval accepts 0 (periodic scan off) and every duration from 15m to 168h
// inclusive; values outside are rejected in TestLoadInvalidValues.
func TestLoadScanIntervalBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value string
		want  time.Duration
	}{
		{value: "0", want: 0},
		{value: "0s", want: 0},
		{value: "15m", want: 15 * time.Minute},
		{value: "1h30m", want: 90 * time.Minute},
		{value: "168h", want: 168 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, "", map[string]string{env("SCAN_INTERVAL"): tt.value})

			got, err := f.load()
			if err != nil {
				t.Fatalf("Load(interval %s): %v", tt.value, err)
			}

			if got.Scan.Interval != tt.want {
				t.Errorf("Scan.Interval = %s, want %s", got.Scan.Interval, tt.want)
			}
		})
	}
}

func TestLoadScanSettingsFromFile(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "scan:\n  watch: FALSE\n  interval: 15m\n", nil)

	got, err := f.load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.Scan.Watch || got.Scan.Interval != 15*time.Minute {
		t.Errorf("Scan = %+v, want watch off and interval 15m", got.Scan)
	}
}
