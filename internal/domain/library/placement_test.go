package library_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

func showLevel() library.Level {
	return library.Level{
		Kind: library.ItemKindShow, MatchKey: "dragon ball super", Title: "Dragon Ball Super",
		Year: library.NumberOf(2015),
	}
}

func seasonLevel(number int) library.Level {
	return library.Level{
		Kind: library.ItemKindSeason, MatchKey: "1", Title: "Season 1", Number: library.NumberOf(number),
	}
}

func episodeLevel() library.Level {
	return library.Level{
		Kind: library.ItemKindEpisode, MatchKey: "130", Title: "Episode 130",
		Number: library.NumberOf(130), AbsoluteNumber: library.NumberOf(130),
	}
}

func movieLevel() library.Level {
	return library.Level{
		Kind: library.ItemKindMovie, MatchKey: "dir:inception 2010", Title: "Inception", Year: library.NumberOf(2010),
	}
}

func extraLevel() library.Level {
	return library.Level{
		Kind: library.ItemKindExtra, MatchKey: "dir:inception 2010|trailers/teaser",
		Title: "Teaser", ExtraType: library.ExtraTypeTrailer,
	}
}

func TestPlacementValidateAcceptsWellFormedPlacements(t *testing.T) {
	t.Parallel()

	multiEpisode := episodeLevel()
	multiEpisode.MatchKey, multiEpisode.Number, multiEpisode.NumberEnd = "1-2", library.NumberOf(1), library.NumberOf(2)

	unnumbered := episodeLevel()
	unnumbered.MatchKey, unnumbered.Number, unnumbered.AbsoluteNumber = "show 2024 01 05", library.OptionalNumber{}, library.OptionalNumber{}

	showExtra := extraLevel()
	showExtra.MatchKey = "dragon ball super|extras/opening"

	tests := []struct {
		name      string
		placement library.Placement
	}{
		{name: "loose movie", placement: library.Placement{Levels: []library.Level{movieLevel()}}},
		{
			name: "movie version and part",
			placement: library.Placement{
				Levels: []library.Level{movieLevel()}, VersionLabel: "4K", PartNumber: library.NumberOf(2),
			},
		},
		{name: "movie extra", placement: library.Placement{Levels: []library.Level{movieLevel(), extraLevel()}}},
		{name: "show extra", placement: library.Placement{Levels: []library.Level{showLevel(), showExtra}}},
		{name: "episode", placement: library.Placement{Levels: []library.Level{showLevel(), seasonLevel(1), episodeLevel()}}},
		{name: "special", placement: library.Placement{Levels: []library.Level{showLevel(), seasonLevel(0), episodeLevel()}}},
		{name: "multi-episode", placement: library.Placement{Levels: []library.Level{showLevel(), seasonLevel(1), multiEpisode}}},
		{name: "unnumbered episode", placement: library.Placement{Levels: []library.Level{showLevel(), seasonLevel(1), unnumbered}}},
		{name: "episode zero", placement: library.Placement{Levels: []library.Level{
			showLevel(), seasonLevel(0), {Kind: library.ItemKindEpisode, MatchKey: "0", Title: "Pilot", Number: library.NumberOf(0)},
		}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if err := test.placement.Validate(); err != nil {
				t.Errorf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestPlacementValidateRejectsMalformedPlacements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*library.Placement)
	}{
		{name: "no level", mutate: func(p *library.Placement) { p.Levels = nil }},
		{name: "show leaf", mutate: func(p *library.Placement) { p.Levels = p.Levels[:1] }},
		{name: "season leaf", mutate: func(p *library.Placement) { p.Levels = p.Levels[:2] }},
		{name: "episode without season", mutate: func(p *library.Placement) { p.Levels = []library.Level{showLevel(), episodeLevel()} }},
		{name: "season at the top", mutate: func(p *library.Placement) { p.Levels = p.Levels[1:] }},
		{name: "episode at the top", mutate: func(p *library.Placement) { p.Levels = p.Levels[2:] }},
		{name: "extra at the top", mutate: func(p *library.Placement) { p.Levels = []library.Level{extraLevel()} }},
		{name: "extra below a season", mutate: func(p *library.Placement) { p.Levels[2] = extraLevel() }},
		{name: "movie below a show", mutate: func(p *library.Placement) { p.Levels = []library.Level{showLevel(), movieLevel()} }},
		{name: "unknown kind", mutate: func(p *library.Placement) { p.Levels[2].Kind = "song" }},
		{name: "empty key", mutate: func(p *library.Placement) { p.Levels[1].MatchKey = "" }},
		{name: "blank key", mutate: func(p *library.Placement) { p.Levels[0].MatchKey = " \t" }},
		{name: "empty title", mutate: func(p *library.Placement) { p.Levels[2].Title = "" }},
		{name: "blank title", mutate: func(p *library.Placement) { p.Levels[0].Title = "  " }},
		{name: "negative year", mutate: func(p *library.Placement) { p.Levels[0].Year = library.NumberOf(-1) }},
		{name: "negative number", mutate: func(p *library.Placement) { p.Levels[1].Number = library.NumberOf(-1) }},
		{name: "negative absolute number", mutate: func(p *library.Placement) { p.Levels[2].AbsoluteNumber = library.NumberOf(-5) }},
		{name: "negative part", mutate: func(p *library.Placement) { p.PartNumber = library.NumberOf(-1) }},
		{name: "end before number", mutate: func(p *library.Placement) { p.Levels[2].NumberEnd = library.NumberOf(129) }},
		{name: "end without number", mutate: func(p *library.Placement) {
			p.Levels[2].Number, p.Levels[2].NumberEnd = library.OptionalNumber{}, library.NumberOf(3)
		}},
		{name: "extra type on an episode", mutate: func(p *library.Placement) { p.Levels[2].ExtraType = library.ExtraTypeTrailer }},
		{name: "extra type on a show", mutate: func(p *library.Placement) { p.Levels[0].ExtraType = library.ExtraTypeOther }},
		{name: "extra without type", mutate: func(p *library.Placement) {
			extra := extraLevel()
			extra.ExtraType = ""
			p.Levels = []library.Level{movieLevel(), extra}
		}},
		{name: "extra with unknown type", mutate: func(p *library.Placement) {
			extra := extraLevel()
			extra.ExtraType = "blooper"
			p.Levels = []library.Level{movieLevel(), extra}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			placement := library.Placement{Levels: []library.Level{showLevel(), seasonLevel(1), episodeLevel()}}
			test.mutate(&placement)

			if err := placement.Validate(); !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("Validate() error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestPlacementKeyIsCanonical(t *testing.T) {
	t.Parallel()

	episode := library.Placement{Levels: []library.Level{showLevel(), seasonLevel(1), episodeLevel()}}
	movie := library.Placement{Levels: []library.Level{movieLevel()}, VersionLabel: "4K", PartNumber: library.NumberOf(2)}

	tests := []struct {
		name      string
		placement library.Placement
		want      string
	}{
		{
			name:      "episode",
			placement: episode,
			want:      `show "dragon ball super"/season "1"/episode "130";version "";part`,
		},
		{
			name:      "movie with version and part",
			placement: movie,
			want:      `movie "dir:inception 2010";version "4K";part 2`,
		},
		{
			name:      "quotes and separators are escaped",
			placement: library.Placement{Levels: []library.Level{{Kind: library.ItemKindMovie, MatchKey: `a"/b;c`, Title: "x"}}},
			want:      `movie "a\"/b;c";version "";part`,
		},
		{name: "empty placement", placement: library.Placement{}, want: `;version "";part`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := test.placement.Key(); got != test.want {
				t.Errorf("Key() = %s, want %s", got, test.want)
			}
		})
	}
}

func TestPlacementKeyIgnoresDisplayFieldsAndSeparatesIdentities(t *testing.T) {
	t.Parallel()

	base := library.Placement{Levels: []library.Level{showLevel(), seasonLevel(1), episodeLevel()}}

	retitled := library.Placement{Levels: []library.Level{showLevel(), seasonLevel(1), episodeLevel()}}
	retitled.Levels[2].Title = "Episode 130 - The Pilot"
	retitled.Levels[0].Year = library.OptionalNumber{}

	if base.Key() != retitled.Key() {
		t.Errorf("titles and numbers changed the key: %s vs %s", base.Key(), retitled.Key())
	}

	distinct := map[string]library.Placement{
		"base":          base,
		"other version": {Levels: base.Levels, VersionLabel: "1080p"},
		"part 1":        {Levels: base.Levels, PartNumber: library.NumberOf(1)},
		"part 0":        {Levels: base.Levels, PartNumber: library.NumberOf(0)},
		"key split":     {Levels: []library.Level{{Kind: library.ItemKindMovie, MatchKey: `a" movie "b`}}},
		"key joined":    {Levels: []library.Level{{Kind: library.ItemKindMovie, MatchKey: "a"}, {Kind: library.ItemKindMovie, MatchKey: "b"}}},
	}

	seen := map[string]string{}

	for name, placement := range distinct {
		key := placement.Key()
		if other, ok := seen[key]; ok {
			t.Errorf("%s and %s share the key %s", name, other, key)
		}

		seen[key] = name
	}
}

func TestOptionalNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		number    library.OptionalNumber
		wantValue int
		wantSet   bool
		wantText  string
	}{
		{name: "absent", number: library.OptionalNumber{}, wantValue: 0, wantSet: false, wantText: ""},
		{name: "zero", number: library.NumberOf(0), wantValue: 0, wantSet: true, wantText: "0"},
		{name: "positive", number: library.NumberOf(131), wantValue: 131, wantSet: true, wantText: "131"},
		{name: "negative", number: library.NumberOf(-2), wantValue: -2, wantSet: true, wantText: "-2"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			value, set := test.number.Get()
			if value != test.wantValue || set != test.wantSet || test.number.IsSet() != test.wantSet {
				t.Errorf("Get() = %d, %v, want %d, %v", value, set, test.wantValue, test.wantSet)
			}

			if got := test.number.String(); got != test.wantText {
				t.Errorf("String() = %q, want %q", got, test.wantText)
			}
		})
	}
}
