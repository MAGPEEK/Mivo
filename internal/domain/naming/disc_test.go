package naming

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

func TestPlaceDiscEqualsVideoFile(t *testing.T) {
	tests := []struct {
		name  string
		kind  library.Kind
		disc  string
		video string
	}{
		{"bdmv folder", library.KindMovies, "Movie (2019)/BDMV", "Movie (2019)/Movie (2019).mkv"},
		{"video_ts lower case", library.KindMovies, "Movie (2019)/video_ts", "Movie (2019)/Movie (2019).mkv"},
		{"nested", library.KindMovies, "Films/Movie (2019)/VIDEO_TS", "Films/Movie (2019)/Movie (2019).mkv"},
		{"iso", library.KindMovies, "Films/Movie (2019).ISO", "Films/Movie (2019).mkv"},
		{"show folder disc", library.KindShows, "Show/Season 1/Show S01E02/VIDEO_TS", "Show/Season 1/Show S01E02.mkv"},
		{"show iso", library.KindShows, "Show/Season 1/Show S01E03.iso", "Show/Season 1/Show S01E03.mkv"},
	}
	for _, test := range tests {
		got, err := PlaceDisc(test.kind, test.disc)
		if err != nil {
			t.Fatalf("%s: %v", test.name, err)
		}

		want, err := Place(test.kind, FileContext{Path: test.video, DirectoryVideoCount: 1})
		if err != nil {
			t.Fatalf("%s: video: %v", test.name, err)
		}

		if got.Outcome != want.Outcome || len(got.Placement.Levels) != len(want.Placement.Levels) {
			t.Fatalf("%s: got %+v want %+v", test.name, got, want)
		}

		for index, level := range got.Placement.Levels {
			if wantLevel := want.Placement.Levels[index]; level != wantLevel {
				t.Errorf("%s: level %d got %+v want %+v", test.name, index, level, wantLevel)
			}
		}
	}
}

func TestPlaceDiscMovieTitle(t *testing.T) {
	result, err := PlaceDisc(library.KindMovies, "Movie (2019)/BDMV")
	if err != nil {
		t.Fatal(err)
	}

	level := result.Placement.Levels[0]
	if level.Title != "Movie" || level.Year.String() != "2019" {
		t.Errorf("level %+v", level)
	}
}

func TestPlaceDiscRejects(t *testing.T) {
	tests := []struct {
		name string
		kind library.Kind
		disc string
	}{
		{"root bdmv", library.KindMovies, "BDMV"},
		{"not a disc", library.KindMovies, "Movie/STREAM"},
		{"bad path", library.KindMovies, "../Movie/BDMV"},
		{"unknown kind", library.Kind("x"), "Movie/BDMV"},
	}
	for _, test := range tests {
		if _, err := PlaceDisc(test.kind, test.disc); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: err=%v", test.name, err)
		}
	}
}
