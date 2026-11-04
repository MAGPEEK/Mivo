package metadata_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

func TestParseRole(t *testing.T) {
	t.Parallel()

	roles := []metadata.Role{
		metadata.RoleActor, metadata.RoleDirector, metadata.RoleWriter,
		metadata.RoleProducer, metadata.RoleGuestStar,
	}

	for _, want := range roles {
		got, err := metadata.ParseRole(want.String())
		if err != nil || got != want {
			t.Errorf("ParseRole(%q) = %q, %v, want %q, nil", want, got, err, want)
		}
	}

	for _, text := range []string{"", "Actor", "guest star", "composer"} {
		got, err := metadata.ParseRole(text)
		if !errors.Is(err, domain.ErrInvalid) || got != "" {
			t.Errorf("ParseRole(%q) = %q, %v, want empty, ErrInvalid", text, got, err)
		}
	}
}

func TestRecordProviderID(t *testing.T) {
	t.Parallel()

	record := metadata.Record{ProviderIDs: []metadata.ProviderID{
		{Provider: metadata.ProviderIMDb, Value: "tt0133093"},
		{Provider: metadata.ProviderTMDB, Value: "603"},
	}}

	tests := []struct {
		provider metadata.Provider
		want     string
		found    bool
	}{
		{provider: metadata.ProviderTMDB, want: "603", found: true},
		{provider: metadata.ProviderIMDb, want: "tt0133093", found: true},
		{provider: metadata.ProviderTVDB},
	}

	for _, test := range tests {
		got, found := record.ProviderID(test.provider)
		if got != test.want || found != test.found {
			t.Errorf("ProviderID(%q) = %q, %t, want %q, %t", test.provider, got, found, test.want, test.found)
		}
	}
}
