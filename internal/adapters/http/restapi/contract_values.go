package restapi

import (
	"fmt"
	"math"
	"time"
)

// Contract value conversions shared by the library, item and playback responses, so every
// endpoint sends a duration, a count or a timestamp the same way.

// timestampPrecision is the precision of every timestamp the API sends. JavaScript dates
// hold milliseconds, and the catalog stores modification times in milliseconds.
const timestampPrecision = time.Millisecond

// durationPrecision is the precision of every duration and position the API sends in
// seconds; finer digits are noise to a player that seeks in milliseconds.
const durationPrecision = time.Millisecond

// durationSeconds sends a duration or position in seconds, rounded to milliseconds.
func durationSeconds(duration time.Duration) float64 {
	return float64(duration.Round(durationPrecision)) / float64(time.Second)
}

// optionalDurationSeconds omits an unknown (zero or negative) duration, as the contract
// says for every optional duration.
func optionalDurationSeconds(duration time.Duration) *float64 {
	if duration <= 0 {
		return nil
	}

	seconds := durationSeconds(duration)

	return &seconds
}

// optionalString omits the empty string, which the use cases use for "none" or "unknown".
func optionalString(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}

// optionalInt32 omits 0, which the domain uses for "not reported", and values the contract's
// int32 cannot carry: they are no real width, level, sample rate or channel count, so they
// are dropped rather than clamped into a wrong one.
func optionalInt32(value int) *int32 {
	if value <= 0 || value > math.MaxInt32 {
		return nil
	}

	converted := int32(value)

	return &converted
}

// optionalInt64 omits 0, which the domain uses for "not reported", and negative values.
func optionalInt64(value int64) *int64 {
	if value <= 0 {
		return nil
	}

	return &value
}

// streamIndex converts a stream index. Clients select audio streams by it, so an index the
// contract cannot carry is an error (a server fault, 500) instead of being clamped or
// dropped into another stream's index.
func streamIndex(index int) (int32, error) {
	if index < 0 || index > math.MaxInt32 {
		return 0, fmt.Errorf("stream index %d out of range", index)
	}

	return int32(index), nil
}

// apiTimestamp converts a time to what the contract sends: UTC with millisecond precision,
// encoded as RFC 3339 by encoding/json.
func apiTimestamp(moment time.Time) time.Time {
	return moment.UTC().Truncate(timestampPrecision)
}

// optionalTimestamp omits the zero time, which means "not yet" in the use cases.
func optionalTimestamp(moment time.Time) *time.Time {
	if moment.IsZero() {
		return nil
	}

	converted := apiTimestamp(moment)

	return &converted
}
