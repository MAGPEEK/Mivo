package access_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
)

func TestLimitForPreset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		preset    access.Preset
		customAge access.AgeBucket
		want      access.ContentLimit
	}{
		{preset: access.PresetYoungChildren, want: mustLimit(t, access.AgeAll, true)},
		{preset: access.PresetChildren, want: mustLimit(t, access.Age6, true)},
		{preset: access.PresetTeens, want: mustLimit(t, access.Age12, true)},
		{preset: access.PresetAdults, want: access.NoLimit()},
		{preset: access.PresetCustom, customAge: access.Age16, want: mustLimit(t, access.Age16, true)},
		{preset: access.PresetChildren, customAge: access.Age18, want: mustLimit(t, access.Age6, true)},
	}

	for _, test := range tests {
		t.Run(string(test.preset), func(t *testing.T) {
			t.Parallel()

			got, err := access.LimitForPreset(test.preset, test.customAge)
			if err != nil || got != test.want {
				t.Errorf("LimitForPreset(%q, %d) = %+v, %v; want %+v", test.preset, test.customAge, got, err, test.want)
			}
		})
	}
}

func TestLimitForPresetRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		preset    access.Preset
		customAge access.AgeBucket
	}{
		{preset: "toddlers"},
		{preset: ""},
		{preset: access.PresetCustom, customAge: 13},
	}

	for _, test := range tests {
		if _, err := access.LimitForPreset(test.preset, test.customAge); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("LimitForPreset(%q, %d) error = %v, want ErrInvalid", test.preset, test.customAge, err)
		}
	}
}

func TestContentLimitPreset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		limit access.ContentLimit
		want  access.Preset
	}{
		{name: "adults", limit: access.NoLimit(), want: access.PresetAdults},
		{name: "young children", limit: mustLimit(t, access.AgeAll, true), want: access.PresetYoungChildren},
		{name: "children", limit: mustLimit(t, access.Age6, true), want: access.PresetChildren},
		{name: "teens", limit: mustLimit(t, access.Age12, true), want: access.PresetTeens},
		{name: "sixteen", limit: mustLimit(t, access.Age16, true), want: access.PresetCustom},
		{name: "teens showing unrated", limit: mustLimit(t, access.Age12, false), want: access.PresetCustom},
		{name: "hide unrated only", limit: access.NoLimit().HidingUnrated(true), want: access.PresetCustom},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := test.limit.Preset(); got != test.want {
				t.Errorf("Preset() = %q, want %q", got, test.want)
			}
		})
	}
}
