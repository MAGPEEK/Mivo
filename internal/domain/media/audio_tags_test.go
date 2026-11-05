package media_test

import (
	"testing"

	"github.com/magpeek/mivo/internal/domain/media"
)

func TestAudioTagsPlacement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tags media.AudioTags
		want media.AudioPlacement
	}{
		{
			name: "album artist wins over artist",
			tags: media.AudioTags{AlbumArtist: "Queen", Artist: "Queen feat. Bowie", Album: "Hot Space", Title: "Under Pressure", Disc: 1, Track: 4, Year: 1982},
			want: media.AudioPlacement{ArtistName: "Queen", TrackArtist: "Queen feat. Bowie", AlbumTitle: "Hot Space", TrackTitle: "Under Pressure", Disc: 1, Track: 4, Year: 1982},
		},
		{
			name: "artist when no album artist",
			tags: media.AudioTags{Artist: " Nina Simone ", Album: "Pastel Blues"},
			want: media.AudioPlacement{ArtistName: "Nina Simone", TrackArtist: "Nina Simone", AlbumTitle: "Pastel Blues", Disc: 1},
		},
		{
			name: "various artists spelling is canonical",
			tags: media.AudioTags{AlbumArtist: "various artists", Artist: "Someone", Album: "Hits"},
			want: media.AudioPlacement{ArtistName: media.VariousArtists, TrackArtist: "Someone", AlbumTitle: "Hits", Disc: 1},
		},
		{
			name: "compilation flag without album artist",
			tags: media.AudioTags{Artist: "Someone", Album: "Hits", Compilation: true},
			want: media.AudioPlacement{ArtistName: media.VariousArtists, TrackArtist: "Someone", AlbumTitle: "Hits", Disc: 1},
		},
		{
			name: "compilation flag keeps an explicit album artist",
			tags: media.AudioTags{AlbumArtist: "Ost Team", Artist: "Someone", Compilation: true},
			want: media.AudioPlacement{ArtistName: "Ost Team", TrackArtist: "Someone", Disc: 1},
		},
		{
			name: "compilation without any artist",
			tags: media.AudioTags{Compilation: true},
			want: media.AudioPlacement{ArtistName: media.VariousArtists, TrackArtist: media.VariousArtists, Disc: 1},
		},
		{
			name: "missing disc is one and negatives are dropped",
			tags: media.AudioTags{Disc: -2, Track: -1, Year: -5},
			want: media.AudioPlacement{Disc: 1},
		},
		{
			name: "disc two kept",
			tags: media.AudioTags{AlbumArtist: "A", Disc: 2, Track: 1},
			want: media.AudioPlacement{ArtistName: "A", TrackArtist: "A", Disc: 2, Track: 1},
		},
		{
			name: "no tags",
			want: media.AudioPlacement{Disc: 1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := test.tags.Placement(); got != test.want {
				t.Errorf("Placement() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestVariousArtistsSpellings(t *testing.T) {
	t.Parallel()

	for _, spelling := range []string{"Various", "VA", "V.A.", "Verschiedene Interpreten", "Verschiedene"} {
		got := media.AudioTags{AlbumArtist: spelling}.Placement().ArtistName
		if got != media.VariousArtists {
			t.Errorf("%q -> %q, want %q", spelling, got, media.VariousArtists)
		}
	}
}

func TestIsVariousArtists(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]bool{" va ": true, "Various Artists": true, "Vanessa": false, "": false} {
		if got := media.IsVariousArtists(name); got != want {
			t.Errorf("IsVariousArtists(%q) = %v, want %v", name, got, want)
		}
	}
}
