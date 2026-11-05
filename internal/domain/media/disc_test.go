package media

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

func TestLayout(t *testing.T) {
	tests := []struct {
		layout Layout
		valid  bool
		disc   bool
	}{
		{LayoutFile, true, false},
		{LayoutVideoTS, true, true},
		{LayoutBDMV, true, true},
		{LayoutISO, true, true},
		{"", false, false},
		{"dvd", false, false},
	}
	for _, test := range tests {
		if test.layout.Valid() != test.valid || test.layout.IsDisc() != test.disc {
			t.Errorf("layout %q: valid=%v disc=%v", test.layout, test.layout.Valid(), test.layout.IsDisc())
		}
	}
}

func TestDiscClipValidate(t *testing.T) {
	tests := []struct {
		name string
		clip DiscClip
		ok   bool
	}{
		{"path", DiscClip{Path: "VTS_01_1.VOB", Duration: time.Second}, true},
		{"extent", DiscClip{Offset: 2048, Length: 100}, true},
		{"extent at zero", DiscClip{Length: 100}, true},
		{"empty", DiscClip{}, false},
		{"both", DiscClip{Path: "a", Length: 1}, false},
		{"negative offset", DiscClip{Offset: -1, Length: 1}, false},
		{"zero length", DiscClip{Offset: 5}, false},
		{"negative duration", DiscClip{Path: "a", Duration: -1}, false},
		{"span start", DiscClip{Path: "a", Start: time.Second, Duration: time.Second}, true},
		{"negative span start", DiscClip{Path: "a", Start: -1}, false},
	}
	for _, test := range tests {
		err := test.clip.Validate()
		if (err == nil) != test.ok || (err != nil && !errors.Is(err, domain.ErrInvalid)) {
			t.Errorf("%s: err=%v", test.name, err)
		}
	}
}

func TestDiscTitle(t *testing.T) {
	title := DiscTitle{{Path: "a", Duration: time.Minute}, {Path: "b", Duration: 2 * time.Minute}}
	if err := title.Validate(); err != nil || title.Duration() != 3*time.Minute {
		t.Fatalf("err=%v duration=%v", err, title.Duration())
	}

	if err := (DiscTitle{}).Validate(); !errors.Is(err, ErrDiscNoTitle) {
		t.Errorf("empty title: %v", err)
	}

	if err := (DiscTitle{{Path: "a"}, {}}).Validate(); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("bad clip: %v", err)
	}
}

func TestDiscErrors(t *testing.T) {
	tests := []struct {
		err  *DiscError
		code string
		base error
	}{
		{ErrDiscEncrypted, "disc_encrypted", domain.ErrUnsupported},
		{ErrDiscUnsupported, "disc_unsupported", domain.ErrUnsupported},
		{ErrDiscNoTitle, "disc_no_title", domain.ErrNotFound},
	}
	for _, test := range tests {
		wrapped := fmt.Errorf("open disc: %w", test.err)
		code, ok := DiscErrorCode(wrapped)

		if !ok || code != test.code || test.err.Error() != test.code {
			t.Errorf("%s: code=%q ok=%v", test.code, code, ok)
		}

		if !errors.Is(wrapped, test.err) || !errors.Is(wrapped, test.base) {
			t.Errorf("%s: errors.Is failed", test.code)
		}

		if errors.Is(wrapped, &DiscError{Code: "other"}) {
			t.Errorf("%s matches another code", test.code)
		}
	}

	if _, ok := DiscErrorCode(domain.ErrNotFound); ok {
		t.Error("plain error has a disc code")
	}
}

func TestLayoutDiscRoot(t *testing.T) {
	tests := []struct {
		layout Layout
		path   string
		want   string
	}{
		{LayoutBDMV, "Movie (2019)/BDMV", "Movie (2019)"},
		{LayoutVideoTS, "Show/Season 1/Show S01E02/VIDEO_TS", "Show/Season 1/Show S01E02"},
		{LayoutVideoTS, "VIDEO_TS", "."},
		{LayoutISO, "Show/Show S01E02.iso", "Show/Show S01E02.iso"},
		{LayoutFile, "Movie.mkv", "Movie.mkv"},
		{"", "Movie.mkv", "Movie.mkv"},
	}
	for _, test := range tests {
		if got := test.layout.DiscRoot(test.path); got != test.want {
			t.Errorf("%q.DiscRoot(%q) = %q, want %q", test.layout, test.path, got, test.want)
		}
	}
}

func TestDiscProbeError(t *testing.T) {
	tests := []struct {
		err   error
		want  ProbeError
		found bool
	}{
		{fmt.Errorf("probe: %w", ErrDiscEncrypted), ProbeErrorDiscEncrypted, true},
		{fmt.Errorf("probe: %w", ErrDiscUnsupported), ProbeErrorDiscUnsupported, true},
		{fmt.Errorf("probe: %w", ErrDiscNoTitle), ProbeErrorDiscNoTitle, true},
		{domain.ErrUnsupported, "", false},
	}
	for _, test := range tests {
		got, found := DiscProbeError(test.err)
		if got != test.want || found != test.found {
			t.Errorf("DiscProbeError(%v) = %q, %v, want %q, %v", test.err, got, found, test.want, test.found)
		}
	}
}
