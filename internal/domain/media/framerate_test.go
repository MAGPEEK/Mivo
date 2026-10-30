package media_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

func TestNewFrameRateReducesTheFraction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		numerator, denominator int
		wantText               string
		wantKnown              bool
	}{
		{name: "ntsc film", numerator: 24000, denominator: 1001, wantText: "24000/1001", wantKnown: true},
		{name: "reducible", numerator: 50, denominator: 2, wantText: "25/1", wantKnown: true},
		{name: "ntsc video reducible", numerator: 60000, denominator: 2002, wantText: "30000/1001", wantKnown: true},
		{name: "integer", numerator: 25, denominator: 1, wantText: "25/1", wantKnown: true},
		{name: "ffprobe unknown", numerator: 0, denominator: 0, wantText: "", wantKnown: false},
		{name: "zero numerator", numerator: 0, denominator: 1, wantText: "", wantKnown: false},
		{name: "zero denominator", numerator: 25, denominator: 0, wantText: "", wantKnown: false},
		{name: "negative numerator", numerator: -25, denominator: 1, wantText: "", wantKnown: false},
		{name: "negative denominator", numerator: 25, denominator: -1, wantText: "", wantKnown: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			rate := media.NewFrameRate(test.numerator, test.denominator)
			if got := rate.String(); got != test.wantText {
				t.Errorf("NewFrameRate(%d, %d).String() = %q, want %q",
					test.numerator, test.denominator, got, test.wantText)
			}

			if got := rate.IsKnown(); got != test.wantKnown {
				t.Errorf("NewFrameRate(%d, %d).IsKnown() = %t, want %t",
					test.numerator, test.denominator, got, test.wantKnown)
			}
		})
	}
}

func TestFrameRateFramesPerSecond(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		rate        media.FrameRate
		wantExact   float64
		wantRounded float64
	}{
		{name: "23.976", rate: media.NewFrameRate(24000, 1001), wantExact: 24000.0 / 1001, wantRounded: 23.976},
		{name: "29.97", rate: media.NewFrameRate(30000, 1001), wantExact: 30000.0 / 1001, wantRounded: 29.97},
		{name: "59.94", rate: media.NewFrameRate(60000, 1001), wantExact: 60000.0 / 1001, wantRounded: 59.94},
		{name: "25", rate: media.NewFrameRate(25, 1), wantExact: 25, wantRounded: 25},
		{name: "half away from zero", rate: media.NewFrameRate(1, 2000), wantExact: 0.0005, wantRounded: 0.001},
		{name: "unknown", rate: media.FrameRate{}, wantExact: 0, wantRounded: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := test.rate.FramesPerSecond(); got != test.wantExact {
				t.Errorf("FramesPerSecond() = %v, want %v", got, test.wantExact)
			}

			if got := test.rate.RoundedFramesPerSecond(); got != test.wantRounded {
				t.Errorf("RoundedFramesPerSecond() = %v, want %v", got, test.wantRounded)
			}
		})
	}
}

func TestFrameRateAccessors(t *testing.T) {
	t.Parallel()

	rate := media.NewFrameRate(48000, 2002)
	if rate.Numerator() != 24000 || rate.Denominator() != 1001 {
		t.Errorf("NewFrameRate(48000, 2002) = %d/%d, want 24000/1001", rate.Numerator(), rate.Denominator())
	}

	if rate != media.NewFrameRate(24000, 1001) {
		t.Error("equal rates compare unequal; the fraction must be stored reduced")
	}
}

func TestParseFrameRateAcceptsFfprobeNotation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		text string
		want media.FrameRate
	}{
		{text: "24000/1001", want: media.NewFrameRate(24000, 1001)},
		{text: "50/2", want: media.NewFrameRate(25, 1)},
		{text: "25", want: media.NewFrameRate(25, 1)},
		{text: " 30/1 ", want: media.NewFrameRate(30, 1)},
		{text: "0/0", want: media.FrameRate{}},
		{text: "0/1", want: media.FrameRate{}},
		{text: "", want: media.FrameRate{}},
	}

	for _, test := range tests {
		t.Run(test.text, func(t *testing.T) {
			t.Parallel()

			got, err := media.ParseFrameRate(test.text)
			if err != nil {
				t.Fatalf("ParseFrameRate(%q) error = %v, want nil", test.text, err)
			}

			if got != test.want {
				t.Errorf("ParseFrameRate(%q) = %q, want %q", test.text, got, test.want)
			}
		})
	}
}

func TestParseFrameRateRejectsMalformedText(t *testing.T) {
	t.Parallel()

	malformed := []string{
		"abc", "24000/", "/1001", "1/2/3", "25.0", "-25/1", "25/-1", "25/0", "1e3/1",
		"99999999999999999999/1",
	}

	for _, text := range malformed {
		t.Run(text, func(t *testing.T) {
			t.Parallel()

			got, err := media.ParseFrameRate(text)
			if !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("ParseFrameRate(%q) error = %v, want ErrInvalid", text, err)
			}

			if got.IsKnown() {
				t.Errorf("ParseFrameRate(%q) = %q on error, want unknown", text, got)
			}
		})
	}
}

func TestFrameRateStringRoundTrips(t *testing.T) {
	t.Parallel()

	for _, rate := range []media.FrameRate{media.NewFrameRate(24000, 1001), media.NewFrameRate(60, 1), {}} {
		parsed, err := media.ParseFrameRate(rate.String())
		if err != nil {
			t.Fatalf("ParseFrameRate(%q) error = %v", rate, err)
		}

		if parsed != rate {
			t.Errorf("ParseFrameRate(%q) = %q, want %q", rate, parsed, rate)
		}
	}
}
