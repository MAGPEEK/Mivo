package metadata_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

func TestFieldsHaveTheStoredTextsOfADR0027(t *testing.T) {
	t.Parallel()

	want := []string{
		"title", "sort_title", "original_title", "year", "release_date", "summary", "tagline",
		"content_rating", "genres", "studios", "countries", "collections", "people",
		"provider_ids", "artwork.poster", "artwork.backdrop", "artwork.thumb", "artwork.logo",
		"artwork.banner",
	}

	fields := metadata.Fields()
	got := make([]string, 0, len(fields))

	for _, field := range fields {
		got = append(got, field.String())
	}

	if !slices.Equal(got, want) {
		t.Errorf("Fields() = %v, want %v", got, want)
	}
}

func TestParseField(t *testing.T) {
	t.Parallel()

	for _, want := range metadata.Fields() {
		got, err := metadata.ParseField(want.String())
		if err != nil || got != want {
			t.Errorf("ParseField(%q) = %q, %v, want %q, nil", want, got, err, want)
		}
	}

	for _, text := range []string{"", "Title", "artwork", "artwork.square", "rating"} {
		got, err := metadata.ParseField(text)
		if !errors.Is(err, domain.ErrInvalid) || got != "" {
			t.Errorf("ParseField(%q) = %q, %v, want empty, ErrInvalid", text, got, err)
		}
	}
}

func TestArtworkField(t *testing.T) {
	t.Parallel()

	for _, kind := range artwork.Kinds() {
		got, err := metadata.ArtworkField(kind)
		if err != nil || got.String() != "artwork."+kind.String() {
			t.Errorf("ArtworkField(%q) = %q, %v, want artwork.%s, nil", kind, got, err, kind)
		}
	}

	got, err := metadata.ArtworkField("square")
	if !errors.Is(err, domain.ErrInvalid) || got != "" {
		t.Errorf("ArtworkField(square) = %q, %v, want empty, ErrInvalid", got, err)
	}
}

func TestLockSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		set    metadata.LockSet
		locked []metadata.Field
	}{
		{name: "zero value", set: metadata.LockSet{}, locked: []metadata.Field{}},
		{name: "empty list", set: metadata.NewLockSet(nil), locked: []metadata.Field{}},
		{
			name: "duplicates and unknown fields",
			set: metadata.NewLockSet([]metadata.Field{
				metadata.FieldArtworkPoster, metadata.FieldTitle, "unknown", metadata.FieldTitle,
			}),
			locked: []metadata.Field{metadata.FieldTitle, metadata.FieldArtworkPoster},
		},
		{name: "all fields", set: metadata.NewLockSet(metadata.Fields()), locked: metadata.Fields()},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			for _, field := range metadata.Fields() {
				if got, want := test.set.Has(field), slices.Contains(test.locked, field); got != want {
					t.Errorf("Has(%q) = %t, want %t", field, got, want)
				}
			}

			if got := test.set.Fields(); !slices.Equal(got, test.locked) {
				t.Errorf("Fields() = %v, want %v", got, test.locked)
			}
		})
	}
}

func TestParseSource(t *testing.T) {
	t.Parallel()

	sources := []metadata.Source{
		metadata.SourceFilename, metadata.SourceNFO, metadata.SourceEmbedded,
		metadata.SourceTMDB, metadata.SourceMusicBrainz, metadata.SourceUser,
	}

	for _, want := range sources {
		got, err := metadata.ParseSource(want.String())
		if err != nil || got != want {
			t.Errorf("ParseSource(%q) = %q, %v, want %q, nil", want, got, err, want)
		}
	}

	for _, text := range []string{"", "NFO", "tvdb", "file name"} {
		got, err := metadata.ParseSource(text)
		if !errors.Is(err, domain.ErrInvalid) || got != "" {
			t.Errorf("ParseSource(%q) = %q, %v, want empty, ErrInvalid", text, got, err)
		}
	}
}
