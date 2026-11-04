package metadata_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

func TestParseProvider(t *testing.T) {
	t.Parallel()

	for _, want := range []metadata.Provider{metadata.ProviderTMDB, metadata.ProviderIMDb, metadata.ProviderTVDB} {
		got, err := metadata.ParseProvider(want.String())
		if err != nil || got != want {
			t.Errorf("ParseProvider(%q) = %q, %v, want %q, nil", want, got, err, want)
		}
	}

	for _, text := range []string{"", "TMDB", "anidb", "imdb "} {
		got, err := metadata.ParseProvider(text)
		if !errors.Is(err, domain.ErrInvalid) || got != "" {
			t.Errorf("ParseProvider(%q) = %q, %v, want empty, ErrInvalid", text, got, err)
		}
	}
}

func TestValidateProviderID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider metadata.Provider
		id       string
		valid    bool
	}{
		{name: "tmdb digits", provider: metadata.ProviderTMDB, id: "603", valid: true},
		{name: "tmdb one digit", provider: metadata.ProviderTMDB, id: "7", valid: true},
		{name: "tmdb 18 digits", provider: metadata.ProviderTMDB, id: "123456789012345678", valid: true},
		{name: "tmdb 19 digits", provider: metadata.ProviderTMDB, id: "1234567890123456789"},
		{name: "tmdb leading zero", provider: metadata.ProviderTMDB, id: "0603"},
		{name: "tmdb zero", provider: metadata.ProviderTMDB, id: "0"},
		{name: "tmdb empty", provider: metadata.ProviderTMDB, id: ""},
		{name: "tmdb letters", provider: metadata.ProviderTMDB, id: "60a"},
		{name: "tmdb negative", provider: metadata.ProviderTMDB, id: "-603"},
		{name: "tmdb arabic digits", provider: metadata.ProviderTMDB, id: "٦٠٣"},
		{name: "tmdb imdb form", provider: metadata.ProviderTMDB, id: "tt0133093"},
		{name: "tvdb digits", provider: metadata.ProviderTVDB, id: "81189", valid: true},
		{name: "tvdb leading zero", provider: metadata.ProviderTVDB, id: "081189"},
		{name: "tvdb space", provider: metadata.ProviderTVDB, id: "81189 "},
		{name: "imdb 7 digits", provider: metadata.ProviderIMDb, id: "tt0133093", valid: true},
		{name: "imdb 8 digits", provider: metadata.ProviderIMDb, id: "tt10872600", valid: true},
		{name: "imdb 10 digits", provider: metadata.ProviderIMDb, id: "tt1234567890", valid: true},
		{name: "imdb 6 digits", provider: metadata.ProviderIMDb, id: "tt013309"},
		{name: "imdb 11 digits", provider: metadata.ProviderIMDb, id: "tt12345678901"},
		{name: "imdb no prefix", provider: metadata.ProviderIMDb, id: "0133093"},
		{name: "imdb upper prefix", provider: metadata.ProviderIMDb, id: "TT0133093"},
		{name: "imdb name ID", provider: metadata.ProviderIMDb, id: "nm0000206"},
		{name: "imdb letters", provider: metadata.ProviderIMDb, id: "tt013309x"},
		{name: "unknown provider", provider: "anidb", id: "123"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := metadata.ValidateProviderID(test.provider, test.id)

			switch {
			case test.valid && err != nil:
				t.Errorf("ValidateProviderID(%q, %q) error = %v, want nil", test.provider, test.id, err)
			case !test.valid && !errors.Is(err, domain.ErrInvalid):
				t.Errorf("ValidateProviderID(%q, %q) error = %v, want ErrInvalid", test.provider, test.id, err)
			}
		})
	}
}
