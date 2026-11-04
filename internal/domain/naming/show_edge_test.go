package naming

import (
	"errors"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

const longTitleLength = 300

func TestPlaceShowEdgeRows(t *testing.T) {
	t.Parallel()

	cases := []showCase{
		// dash heuristic and resolutions
		{"Show/Show 05 - 1080.mkv", "show", 1, "5", "1080", ""},
		{"Show/Show - 3 Days - 05.mkv", "show", 1, "5", "Episode 5", ""},
		{"Show/Show - 1080.mkv", "show", 1, "1080", "Episode 1080", ""},
		{"Show/Show - 05 - 3 Days.mkv", "show", 1, "5", "3 Days", ""},
		// season folders with suffixes
		{"Show/Season 2 [1080p]/Show - 04.mkv", "show", 2, "4", "Episode 4", ""},
		{"Show/Season 1 (2020)/Show - 04.mkv", "show", 1, "4", "Episode 4", ""},
		{"Show/Staffel 3 (2020)/E04.mkv", "show", 3, "4", "Episode 4", ""},
		// non-Latin scripts
		{"[Group] 作品 - 05.mkv", "name:作品", 1, "5", "Episode 5", ""},
		{"作品/作品 - 05.mkv", "作品", 1, "5", "Episode 5", ""},
		// name without a stem
		{"Show/.mkv", "show", 1, "title:", "Episode", ""},
	}

	for _, testCase := range cases {
		t.Run(testCase.path, func(t *testing.T) {
			t.Parallel()

			testCase.check(t, placeShow(t, testCase.path))
		})
	}
}

func TestPlaceShowExplicitMarkerIsNotAbsolute(t *testing.T) {
	t.Parallel()

	for _, filePath := range []string{"Show/Season 2/E04.mkv", "Show/Season 2/Ep04.mkv", "Show/Season 2/Episode 4.mkv"} {
		if absolute := placeShow(t, filePath).Levels[2].AbsoluteNumber; absolute.IsSet() {
			t.Errorf("%q carries an absolute number", filePath)
		}
	}
}

func TestPlaceShowVeryLongName(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", longTitleLength)
	levels := placeShow(t, "Show/"+long+" - 05.mkv").Levels

	if levels[2].MatchKey != "5" {
		t.Errorf("episode key = %q, want 5", levels[2].MatchKey)
	}
}

func TestPlaceShowUnkeyableFolders(t *testing.T) {
	t.Parallel()

	for _, filePath := range []string{"!!!/Show - 05.mkv", "[]/Show - 05.mkv", "(--)/a.mkv", ".mkv"} {
		_, err := Place(library.KindShows, FileContext{Path: filePath})
		if !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("Place(%q) error = %v, want ErrInvalid", filePath, err)
		}
	}
}

// TestPlaceShowKeyStability checks what rescans rely on: the placement key follows the
// episode number, not the episode title, and a version differs only in its label.
func TestPlaceShowKeyStability(t *testing.T) {
	t.Parallel()

	key := func(filePath string) library.Placement { return placeShow(t, filePath) }

	first := key("Show/Season 1/Show - S01E01.mkv")
	renamed := key("Show/Season 1/Show - S01E01 - Pilot.mkv")
	other := key("Show/Season 1/Show - S01E01 - Another Title.mkv")

	if first.Key() != renamed.Key() || renamed.Key() != other.Key() {
		t.Errorf("keys differ: %q %q %q", first.Key(), renamed.Key(), other.Key())
	}

	version := key("Show/Season 1/Show - S01E01 - Pilot - Extended.mkv")
	if version.Levels[2].MatchKey != renamed.Levels[2].MatchKey || version.VersionLabel == renamed.VersionLabel {
		t.Errorf("version = %+v, base = %+v", version, renamed)
	}

	if a, b := key("Show/Show - 05.mkv"), key("Show/Show - 06.mkv"); a.Key() == b.Key() {
		t.Error("different episode numbers share a key")
	}
}
