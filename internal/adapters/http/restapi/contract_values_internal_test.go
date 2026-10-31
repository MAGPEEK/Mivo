package restapi

import (
	"math"
	"strconv"
	"testing"
	"time"
)

func TestDurationSecondsRoundsToMilliseconds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		duration time.Duration
		want     float64
	}{
		{duration: 0, want: 0},
		{duration: 598500 * time.Millisecond, want: 598.5},
		{duration: 598*time.Second + 500400*time.Microsecond, want: 598.5},
		{duration: 598*time.Second + 500500*time.Microsecond, want: 598.501},
		{duration: time.Nanosecond, want: 0},
	}

	for _, test := range tests {
		if got := durationSeconds(test.duration); got != test.want {
			t.Errorf("durationSeconds(%v) = %v, want %v", test.duration, got, test.want)
		}
	}
}

func TestOptionalDurationSecondsOmitsUnknown(t *testing.T) {
	t.Parallel()

	for _, unknown := range []time.Duration{0, -time.Second} {
		if got := optionalDurationSeconds(unknown); got != nil {
			t.Errorf("optionalDurationSeconds(%v) = %v, want nil", unknown, *got)
		}
	}

	if got := optionalDurationSeconds(5423042 * time.Millisecond); got == nil || *got != 5423.042 {
		t.Errorf("optionalDurationSeconds(5423.042s) = %v, want 5423.042", got)
	}
}

func TestOptionalInt32(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value int
		want  *int32
	}{
		{value: -1},
		{value: 0},
		{value: 1, want: new(int32(1))},
		{value: math.MaxInt32, want: new(int32(math.MaxInt32))},
	}

	for _, test := range tests {
		got := optionalInt32(test.value)
		if (got == nil) != (test.want == nil) || (got != nil && *got != *test.want) {
			t.Errorf("optionalInt32(%d) = %v, want %v", test.value, got, test.want)
		}
	}
}

func TestOptionalInt64(t *testing.T) {
	t.Parallel()

	for _, omitted := range []int64{math.MinInt64, -1, 0} {
		if got := optionalInt64(omitted); got != nil {
			t.Errorf("optionalInt64(%d) = %d, want nil", omitted, *got)
		}
	}

	if got := optionalInt64(math.MaxInt64); got == nil || *got != math.MaxInt64 {
		t.Errorf("optionalInt64(MaxInt64) = %v, want MaxInt64", got)
	}
}

func TestStreamIndex(t *testing.T) {
	t.Parallel()

	for _, valid := range []int{0, 1, math.MaxInt32} {
		if got, err := streamIndex(valid); err != nil || int(got) != valid {
			t.Errorf("streamIndex(%d) = %d, %v; want %d", valid, got, err, valid)
		}
	}

	for _, invalid := range append([]int{-1, math.MinInt32}, beyondInt32()...) {
		if _, err := streamIndex(invalid); err == nil {
			t.Errorf("streamIndex(%d) succeeded, want an error", invalid)
		}
	}
}

func TestOptionalString(t *testing.T) {
	t.Parallel()

	if got := optionalString(""); got != nil {
		t.Errorf(`optionalString("") = %q, want nil`, *got)
	}

	if got := optionalString("x"); got == nil || *got != "x" {
		t.Errorf(`optionalString("x") = %v, want "x"`, got)
	}
}

// TestOptionalInt32DropsValuesBeyondInt32 covers the ints only a 64-bit platform can hold.
func TestOptionalInt32DropsValuesBeyondInt32(t *testing.T) {
	t.Parallel()

	for _, value := range beyondInt32() {
		if got := optionalInt32(value); got != nil {
			t.Errorf("optionalInt32(%d) = %d, want nil", value, *got)
		}
	}
}

// beyondInt32 returns ints the contract's int32 cannot carry; empty where int is 32 bits
// wide (GOARCH=arm), because such values cannot exist there. The conversion goes through a
// variable so the file compiles on every platform.
func beyondInt32() []int {
	if strconv.IntSize < int64Bits {
		return nil
	}

	above, below := int64(math.MaxInt32)+1, int64(math.MinInt32)-1

	return []int{int(above), int(below)}
}

// int64Bits is the width of int on 64-bit platforms.
const int64Bits = 64
