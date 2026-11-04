package naming

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

func TestPlaceInPhotos(t *testing.T) {
	t.Parallel()

	album := func(key, title string) library.Level {
		return library.Level{Kind: library.ItemKindPhotoAlbum, MatchKey: key, Title: title}
	}
	photoLevel := func(key, title string) library.Level {
		return library.Level{Kind: library.ItemKindPhoto, MatchKey: key, Title: title}
	}

	cases := []struct {
		name string
		path string
		want []library.Level
	}{
		{"root photo", "IMG_0001.JPG", []library.Level{photoLevel("img 0001 jpg", "IMG 0001")}},
		{"album", "Holiday/IMG_0001.jpg", []library.Level{
			album("holiday", "Holiday"), photoLevel("img 0001 jpg", "IMG 0001"),
		}},
		{"nested albums", "2020/Summer Trip/a.heic", []library.Level{
			album("2020", "2020"), album("summer trip", "Summer Trip"), photoLevel("a heic", "a"),
		}},
		{"video in photo library", "Clips/day.mp4", []library.Level{
			album("clips", "Clips"), photoLevel("day mp4", "day"),
		}},
		{"symbol-only album", "!!!/a.png", []library.Level{
			album("name:!!!", "!!!"), photoLevel("a png", "a"),
		}},
	}

	for _, testCase := range cases {
		result, err := Place(library.KindPhotos, FileContext{Path: testCase.path})
		if err != nil {
			t.Fatalf("%s: error = %v", testCase.name, err)
		}

		if result.Outcome != OutcomePlaced {
			t.Errorf("%s: outcome = %q", testCase.name, result.Outcome)
		}

		assertPlacement(t, result.Placement, library.Placement{Levels: testCase.want})
	}
}

// TestPlaceInPhotosKeyCollision pins the documented limit: punctuation differences vanish
// in the match key.
func TestPlaceInPhotosKeyCollision(t *testing.T) {
	t.Parallel()

	first, err := Place(library.KindPhotos, FileContext{Path: "a.b.jpg"})
	if err != nil {
		t.Fatal(err)
	}

	second, err := Place(library.KindPhotos, FileContext{Path: "a b.jpg"})
	if err != nil {
		t.Fatal(err)
	}

	if first.Placement.Key() != second.Placement.Key() {
		t.Errorf("keys differ: %s vs %s", first.Placement.Key(), second.Placement.Key())
	}
}

func TestPlaceInPhotosRejects(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"", "/abs/a.jpg", "../a.jpg", "a/ /b.jpg"} {
		_, err := Place(library.KindPhotos, FileContext{Path: path})
		if !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("Place(%q) error = %v, want ErrInvalid", path, err)
		}
	}
}
