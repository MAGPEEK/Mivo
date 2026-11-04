package naming_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/naming"
)

type wantLevel struct {
	title  string
	key    string
	year   int
	number int
}

func TestPlaceMusic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		path   string
		tags   media.AudioTags
		artist wantLevel
		album  wantLevel
		track  wantLevel
	}{
		{
			name:   "folders only",
			path:   "Pink Floyd/The Wall (1979)/05 Another Brick.flac",
			artist: wantLevel{title: "Pink Floyd", key: "pink floyd"},
			album:  wantLevel{title: "The Wall", key: "pink floyd/the wall/1979", year: 1979},
			track:  wantLevel{title: "Another Brick", key: "d1t5 another brick", number: 5},
		},
		{
			name:   "disc folder",
			path:   "Pink Floyd/The Wall/CD 2/03 Hey You.flac",
			artist: wantLevel{title: "Pink Floyd", key: "pink floyd"},
			album:  wantLevel{title: "The Wall", key: "pink floyd/the wall/"},
			track:  wantLevel{title: "Hey You", key: "d2t3 hey you", number: 3},
		},
		{
			name:   "disc and track in the file name",
			path:   "A/B/2-07 Song_Name.mp3",
			artist: wantLevel{title: "A", key: "a"},
			album:  wantLevel{title: "B", key: "a/b/"},
			track:  wantLevel{title: "Song Name", key: "d2t7 song name", number: 7},
		},
		{
			name:   "dash separated track number and genre folder above",
			path:   "Rock/Queen/Jazz/01 - Mustapha.m4a",
			artist: wantLevel{title: "Queen", key: "queen"},
			album:  wantLevel{title: "Jazz", key: "queen/jazz/"},
			track:  wantLevel{title: "Mustapha", key: "d1t1 mustapha", number: 1},
		},
		{
			name:   "no track number keeps the title",
			path:   "A/B/Intro.ogg",
			artist: wantLevel{title: "A", key: "a"},
			album:  wantLevel{title: "B", key: "a/b/"},
			track:  wantLevel{title: "Intro", key: "d1 intro"},
		},
		{
			name:   "number-only name is the title",
			path:   "A/B/1999.flac",
			artist: wantLevel{title: "A", key: "a"},
			album:  wantLevel{title: "B", key: "a/b/"},
			track:  wantLevel{title: "1999", key: "d1 1999"},
		},
		{
			name:   "album folder only",
			path:   "B/04 Song.flac",
			artist: wantLevel{title: "Unknown Artist", key: "unknown artist"},
			album:  wantLevel{title: "B", key: "unknown artist/b/"},
			track:  wantLevel{title: "Song", key: "d1t4 song", number: 4},
		},
		{
			name:   "file in root",
			path:   "Song.flac",
			artist: wantLevel{title: "Unknown Artist", key: "unknown artist"},
			album:  wantLevel{title: "Unknown Album", key: "unknown artist/unknown album/"},
			track:  wantLevel{title: "Song", key: "d1 song"},
		},
		{
			name:   "disc folder directly below the artist",
			path:   "A/Disc 1/02 S.flac",
			artist: wantLevel{title: "Unknown Artist", key: "unknown artist"},
			album:  wantLevel{title: "A", key: "unknown artist/a/"},
			track:  wantLevel{title: "S", key: "d1t2 s", number: 2},
		},
		{
			name:   "tags win over folders",
			path:   "Folder Artist/Folder Album (1999)/01 Folder Title.flac",
			tags:   media.AudioTags{AlbumArtist: "Tag Artist", Album: "Tag Album", Title: "Tag Title", Disc: 3, Track: 9, Year: 2001},
			artist: wantLevel{title: "Tag Artist", key: "tag artist"},
			album:  wantLevel{title: "Tag Album", key: "tag artist/tag album/2001", year: 2001},
			track:  wantLevel{title: "Tag Title", key: "d3t9 tag title", number: 9},
		},
		{
			name:   "tags fill field by field",
			path:   "Folder Artist/Folder Album (1999)/CD2/07 Folder Title.flac",
			tags:   media.AudioTags{Title: "Tag Title"},
			artist: wantLevel{title: "Folder Artist", key: "folder artist"},
			album:  wantLevel{title: "Folder Album", key: "folder artist/folder album/1999", year: 1999},
			track:  wantLevel{title: "Tag Title", key: "d2t7 tag title", number: 7},
		},
		{
			name:   "compilation stays one album",
			path:   "Hits/05 X.mp3",
			tags:   media.AudioTags{Artist: "Single Artist", Album: "Hits", Compilation: true, Year: 2020},
			artist: wantLevel{title: "Various Artists", key: "various artists"},
			album:  wantLevel{title: "Hits", key: "various artists/hits/2020", year: 2020},
			track:  wantLevel{title: "X", key: "d1t5 x", number: 5},
		},
		{
			name:   "punctuation-only artist",
			path:   "!!!/Album/01 A.flac",
			artist: wantLevel{title: "!!!", key: "!!!"},
			album:  wantLevel{title: "Album", key: "!!!/album/"},
			track:  wantLevel{title: "A", key: "d1t1 a", number: 1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			placement, err := naming.PlaceMusic(test.path, test.tags)
			if err != nil {
				t.Fatalf("PlaceMusic: %v", err)
			}

			kinds := []library.ItemKind{library.ItemKindArtist, library.ItemKindAlbum, library.ItemKindTrack}
			wants := []wantLevel{test.artist, test.album, test.track}

			if len(placement.Levels) != len(kinds) {
				t.Fatalf("levels = %d", len(placement.Levels))
			}

			for index, level := range placement.Levels {
				assertLevel(t, level, kinds[index], wants[index])
			}
		})
	}
}

func assertLevel(t *testing.T, level library.Level, kind library.ItemKind, want wantLevel) {
	t.Helper()

	if level.Kind != kind || level.Title != want.title || level.MatchKey != want.key {
		t.Errorf("%s level = %q %q, want %q %q", kind, level.Title, level.MatchKey, want.title, want.key)
	}

	year, _ := level.Year.Get()
	number, _ := level.Number.Get()

	if year != want.year || number != want.number {
		t.Errorf("%s year/number = %d/%d, want %d/%d", kind, year, number, want.year, want.number)
	}
}

func TestPlaceMusicRejectsInvalidPaths(t *testing.T) {
	t.Parallel()

	for _, bad := range []string{"", "/abs/a.flac", "../a.flac", "a/../../b.flac"} {
		if _, err := naming.PlaceMusic(bad, media.AudioTags{}); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("PlaceMusic(%q) err = %v, want ErrInvalid", bad, err)
		}
	}
}

func TestPlaceRoutesMusicToPlaceMusic(t *testing.T) {
	t.Parallel()

	_, err := naming.Place(library.KindMusic, naming.FileContext{Path: "A/B/01 S.flac"})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("Place(music) err = %v, want ErrInvalid", err)
	}
}

func TestPlaceMusicDiscSpellings(t *testing.T) {
	t.Parallel()

	for folder, disc := range map[string]string{"CD1": "d1", "cd 2": "d2", "Disc-3": "d3", "disk_4": "d4", "Disc 10": "d10"} {
		placement, err := naming.PlaceMusic("A/B/"+folder+"/01 S.flac", media.AudioTags{})
		if err != nil {
			t.Fatalf("%s: %v", folder, err)
		}

		if got := placement.Levels[2].MatchKey; got[:len(disc)+1] != disc+"t" {
			t.Errorf("%s: track key %q, want prefix %s", folder, got, disc)
		}
	}
}
