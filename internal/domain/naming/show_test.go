package naming

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

type showCase struct {
	path    string
	show    string
	season  int
	episode string
	title   string
	label   string
}

func placeShow(t *testing.T, filePath string) library.Placement {
	t.Helper()

	result, err := Place(library.KindShows, FileContext{Path: filePath, DirectoryVideoCount: 1})
	if err != nil {
		t.Fatalf("Place(%q) error = %v", filePath, err)
	}

	if result.Outcome != OutcomePlaced {
		t.Fatalf("Place(%q) outcome = %q, want placed", filePath, result.Outcome)
	}

	return result.Placement
}

func TestPlaceShowEpisodes(t *testing.T) {
	t.Parallel()

	cases := []showCase{
		// explicit markers
		{"Show/Season 1/Show - S01E02 - Pilot.mkv", "show", 1, "2", "Pilot", ""},
		{"Show/s1e2.mkv", "show", 1, "2", "Episode 2", ""},
		{"Show/Show S2015E01.mkv", "show", 2015, "1", "Episode 1", ""},
		{"Show/S01E01-E02.mkv", "show", 1, "1-2", "Episode 1", ""},
		{"Show/S01E01E02.mkv", "show", 1, "1-2", "Episode 1", ""},
		{"Show/S01E01-02.mkv", "show", 1, "1-2", "Episode 1", ""},
		{"Show/S01E03-E01.mkv", "show", 1, "3", "Episode 3", ""},
		{"Show/Show 1x02.mkv", "show", 1, "2", "Episode 2", ""},
		{"Show/Show 2x05-2x06.mkv", "show", 2, "5-6", "Episode 5", ""},
		{"Show/Show 2x05-06.mkv", "show", 2, "5-6", "Episode 5", ""},
		{"Show/E02.mkv", "show", 1, "2", "Episode 2", ""},
		{"Show/Season 3/Ep02.mkv", "show", 3, "2", "Episode 2", ""},
		{"Show/Season 3/E02-E03.mkv", "show", 3, "2-3", "Episode 2", ""},
		{"Show/Season 2/Episode 2 - Title.mkv", "show", 2, "2", "Title", ""},
		{"Show/Season 2/Episode 2-3.mkv", "show", 2, "2-3", "Episode 2", ""},
		{"Show/Episode.mkv", "show", 1, "title:episode", "Episode", ""},
		{"Show/Season 2/Show - S01E04.mkv", "show", 1, "4", "Episode 4", ""},
		{"Show/Show.S01E01.1080p.WEB-DL.x264-GRP.mkv", "show", 1, "1", "Episode 1", ""},
		{"Show/Show.S01E02.The.Title.720p.HDTV.x264-GRP.mkv", "show", 1, "2", "The Title", ""},
		{"Show/Show - S01E01 - Pilot - Extended.mkv", "show", 1, "1", "Pilot", "Extended"},
		// season folders
		{"Show/Season 01/Show - S01E03.mkv", "show", 1, "3", "Episode 3", ""},
		{"Show/Staffel 2/Show - 05.mkv", "show", 2, "5", "Episode 5", ""},
		{"Show/S2/Show - 06.mkv", "show", 2, "6", "Episode 6", ""},
		{"Show/Specials/Show - S00E01.mkv", "show", 0, "1", "Episode 1", ""},
		{"Show/Specials/Making Of.mkv", "show", 0, "title:making of", "Making Of", ""},
		{"Show/Season 0/Show - 03.mkv", "show", 0, "3", "Episode 3", ""},
		{"Show/Extra Disc/Show - 05.mkv", "show", 1, "5", "Episode 5", ""},
		// absolute numbering and non-numbers
		{"Show/Show - 105.mkv", "show", 1, "105", "Episode 105", ""},
		{"Show/Show 105.mkv", "show", 1, "105", "Episode 105", ""},
		{"Show/Season 2/Show - 105.mkv", "show", 2, "105", "Episode 105", ""},
		{"Show/Show - 05v2.mkv", "show", 1, "5", "Episode 5", ""},
		{"Show/Show - 05 [ABCD1234].mkv", "show", 1, "5", "Episode 5", ""},
		{"Show/Show - 05 [1080p].mkv", "show", 1, "5", "Episode 5", ""},
		{"Show/Show - 05 x264.mkv", "show", 1, "5", "Episode 5", ""},
		{"Show/Show (2015) - 07.mkv", "show", 1, "7", "Episode 7", ""},
		{"Show/Show 2 - 05 - Title.mkv", "show", 1, "5", "Title", ""},
		{"Show/[Group] Show - 08 [720p].mkv", "show", 1, "8", "Episode 8", ""},
		// no marker
		{"Show/Show 720p.mkv", "show", 1, "title:show 720p", "Show", ""},
		{"Show/Show 2024.mkv", "show", 1, "title:show 2024", "Show", ""},
		{"Show/Show 2024-01-05.mkv", "show", 1, "title:show 2024 01 05", "Show 2024-01-05", ""},
		{"Show/Season 4/Behind.mkv", "show", 4, "title:behind", "Behind", ""},
		// root-level files
		{"Show - S01E02 - Pilot.mkv", "name:show", 1, "2", "Pilot", ""},
		{"Show.Name.2x03.mkv", "name:show name", 2, "3", "Episode 3", ""},
		{"Show - 05.mkv", "name:show", 1, "5", "Episode 5", ""},
		{"Show 2024-01-05.mkv", "name:show", 1, "title:show 2024 01 05", "Show 2024-01-05", ""},
		{"Show (2015) S01E01.mkv", "name:show", 1, "1", "Episode 1", ""},
		{"S01E01.mkv", "name:s01e01", 1, "1", "Episode 1", ""},
		// deeper folders
		{"Show/Season 1/Disc 1/Show - 01.mkv", "show", 1, "1", "Episode 1", ""},
		{"Show (2015)/Season 2/Show - S02E01.mkv", "show 2015", 2, "1", "Episode 1", ""},
	}

	for _, testCase := range cases {
		t.Run(testCase.path, func(t *testing.T) {
			t.Parallel()

			testCase.check(t, placeShow(t, testCase.path))
		})
	}
}

func (testCase showCase) check(t *testing.T, placement library.Placement) {
	t.Helper()

	if len(placement.Levels) != 3 {
		t.Fatalf("levels = %d, want 3", len(placement.Levels))
	}

	show, season, episode := placement.Levels[0], placement.Levels[1], placement.Levels[2]

	if show.MatchKey != testCase.show {
		t.Errorf("show key = %q, want %q", show.MatchKey, testCase.show)
	}

	if number, _ := season.Number.Get(); number != testCase.season || season.MatchKey != strconv.Itoa(testCase.season) {
		t.Errorf("season = %d (%q), want %d", number, season.MatchKey, testCase.season)
	}

	if episode.MatchKey != testCase.episode || episode.Title != testCase.title {
		t.Errorf("episode = %q %q, want %q %q", episode.MatchKey, episode.Title, testCase.episode, testCase.title)
	}

	if placement.VersionLabel != testCase.label {
		t.Errorf("label = %q, want %q", placement.VersionLabel, testCase.label)
	}
}

func TestPlaceShowLevelDetails(t *testing.T) {
	t.Parallel()

	placement := placeShow(t, "Name (2015)/Season 1/Name - 105.mkv")

	show, season, episode := placement.Levels[0], placement.Levels[1], placement.Levels[2]

	if year, _ := show.Year.Get(); show.Title != "Name" || year != 2015 || show.MatchKey != "name 2015" {
		t.Errorf("show = %+v", show)
	}

	if season.Title != "Season 1" || season.Kind != library.ItemKindSeason {
		t.Errorf("season = %+v", season)
	}

	if absolute, _ := episode.AbsoluteNumber.Get(); absolute != 105 {
		t.Errorf("absolute = %d, want 105", absolute)
	}

	if end := placeShow(t, "Show/S01E01-E02.mkv").Levels[2].NumberEnd; !end.IsSet() {
		t.Error("multi-episode file lost its end number")
	}

	if title := placeShow(t, "Show/Specials/Show - 01.mkv").Levels[1].Title; title != "Specials" {
		t.Errorf("season 0 title = %q, want Specials", title)
	}

	if absolute := placeShow(t, "Show/S01E05.mkv").Levels[2].AbsoluteNumber; absolute.IsSet() {
		t.Error("SxxEyy episode must not carry an absolute number")
	}

	root := placeShow(t, "Show (2015) S01E01.mkv").Levels[0]
	if year, _ := root.Year.Get(); year != 2015 || root.Title != "Show" {
		t.Errorf("root show = %+v", root)
	}
}

func TestPlaceShowExtrasAndSamples(t *testing.T) {
	t.Parallel()

	extras := []struct {
		path      string
		extraType library.ExtraType
	}{
		{"Show/Extras/Making Of.mkv", library.ExtraTypeOther},
		{"Show/Season 1/Featurettes/Cast.mkv", library.ExtraTypeFeaturette},
		{"Show/trailers/Teaser.mkv", library.ExtraTypeTrailer},
		{"Show/Behind The Scenes/Bloopers.mkv", library.ExtraTypeBehindTheScenes},
	}

	for _, testCase := range extras {
		placement := placeShow(t, testCase.path)
		if len(placement.Levels) != 2 || placement.Levels[0].Kind != library.ItemKindShow {
			t.Fatalf("%q levels = %+v", testCase.path, placement.Levels)
		}

		extra := placement.Levels[1]
		if extra.Kind != library.ItemKindExtra || extra.ExtraType != testCase.extraType {
			t.Errorf("%q extra = %+v", testCase.path, extra)
		}
	}

	for _, filePath := range []string{"Show/Sample/a.mkv", "Show/Show - 01-sample.mkv", "sample.mkv"} {
		result, err := Place(library.KindShows, FileContext{Path: filePath})
		if err != nil || result.Outcome != OutcomeIgnored {
			t.Errorf("Place(%q) = %+v, %v; want ignored", filePath, result, err)
		}
	}
}

func TestPlaceShowErrors(t *testing.T) {
	t.Parallel()

	for _, filePath := range []string{"/a.mkv", "../a.mkv", " /S01E01.mkv"} {
		_, err := Place(library.KindShows, FileContext{Path: filePath})
		if !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("Place(%q) error = %v, want ErrInvalid", filePath, err)
		}
	}
}

// TestAbsoluteNumberingDragonBallSuper is the regression of the Plex walkthrough defect
// (UX DT-24): 131 absolute-numbered files are one show, one season and 131 episodes.
func TestAbsoluteNumberingDragonBallSuper(t *testing.T) {
	t.Parallel()

	const episodeCount = 131

	shows := map[string]bool{}
	seasons := map[string]bool{}
	episodes := map[string]bool{}

	for number := 1; number <= episodeCount; number++ {
		filePath := fmt.Sprintf("Dragon Ball Super/Dragon Ball Super - %03d.mkv", number)
		levels := placeShow(t, filePath).Levels

		shows[levels[0].MatchKey] = true
		seasons[levels[1].MatchKey] = true
		episodes[levels[2].MatchKey] = true

		if got, _ := levels[2].Number.Get(); got != number {
			t.Errorf("%s: episode = %d, want %d", filePath, got, number)
		}
	}

	if len(shows) != 1 || len(seasons) != 1 || !seasons["1"] || len(episodes) != episodeCount {
		t.Errorf("shows=%d seasons=%v episodes=%d, want 1 show, season 1, %d episodes",
			len(shows), seasons, len(episodes), episodeCount)
	}
}

func TestAbsoluteNumberingVariants(t *testing.T) {
	t.Parallel()

	variants := []string{
		"[Group] Dragon Ball Super - 130 [1080p] [ABCD1234].mkv",
		"Dragon Ball Super 130.mkv",
		"Dragon.Ball.Super.E130.mkv",
		"Dragon Ball Super - Episode 130.mkv",
		"Dragon Ball Super (2015)/Dragon Ball Super - 130.mkv",
		"Dragon Ball Super Fansub/[Group] Dragon Ball Super - 130 [1080p] [ABCD1234].mkv",
	}

	seasons := map[string]bool{}

	for _, filePath := range variants {
		levels := placeShow(t, filePath).Levels

		seasonNumber, _ := levels[1].Number.Get()
		episodeNumber, _ := levels[2].Number.Get()

		if seasonNumber != 1 || episodeNumber != 130 {
			t.Errorf("%q = season %d episode %d, want season 1 episode 130", filePath, seasonNumber, episodeNumber)
		}

		// Only bare numbers are absolute; E130 / Episode 130 are explicit markers.
		explicit := strings.Contains(filePath, "E130") || strings.Contains(filePath, "Episode")
		if absolute, set := levels[2].AbsoluteNumber.Get(); set == explicit || (set && absolute != 130) {
			t.Errorf("%q absolute = %d (set %v), want set=%v", filePath, absolute, set, !explicit)
		}

		seasons[levels[1].MatchKey] = true
	}

	if len(seasons) != 1 {
		t.Errorf("distinct season keys = %v, want exactly season 1", seasons)
	}
}
