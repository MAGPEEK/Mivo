package match

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

const epsilon = 1e-9

func tmdbMovie(id string) ProviderRef {
	return ProviderRef{Provider: RefTMDB, Kind: RefKindMovie, ID: id}
}

func TestParseMatchState(t *testing.T) {
	for _, state := range []MatchState{MatchStateMatched, MatchStateUnmatched, MatchStateDisabled} {
		got, err := ParseMatchState(state.String())
		if err != nil || got != state {
			t.Errorf("ParseMatchState(%q) = %q, %v", state, got, err)
		}
	}

	for _, text := range []string{"", "Matched", "manual", "needs_review"} {
		if _, err := ParseMatchState(text); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("ParseMatchState(%q) error = %v, want ErrInvalid", text, err)
		}
	}
}

func TestNormalizeTitle(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"case", "THE MATRIX", "matrix"},
		{"punctuation", "Spider-Man: No Way Home!", "spider man no way home"},
		{"ampersand", "Fast & Furious", "fast and furious"},
		{"and word", "Fast and Furious", "fast and furious"},
		{"article the", "The Matrix", "matrix"},
		{"article a", "A Quiet Place", "quiet place"},
		{"article an", "An American Werewolf", "american werewolf"},
		{"article der", "Der Untergang", "untergang"},
		{"article die", "Die Welle", "welle"},
		{"article das", "Das Boot", "boot"},
		{"article le", "Le Mans", "mans"},
		{"article la", "La Haine", "haine"},
		{"only an article", "The", "the"},
		{"article inside", "Into the Wild", "into the wild"},
		{"umlauts kept", "Märchen über Müller", "märchen über müller"},
		{"eszett folded", "Straße", "strasse"},
		{"accents kept", "Léon", "léon"},
		{"decomposed accents composed", "Léon", "léon"},
		{"decomposed umlaut", "Müller", "müller"},
		{"decomposed unknown base", "x́", "x́"},
		{"cjk", "千と千尋の神隠し", "千と千尋の神隠し"},
		{"cjk with punctuation", "君の名は。", "君の名は"},
		{"spacing", "  x   y  ", "x y"},
		{"empty", "", ""},
		{"stray mark first", "́a", "́a"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := NormalizeTitle(test.in); got != test.want {
				t.Errorf("NormalizeTitle(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

func TestNormalizeTitleComposedEqualsDecomposed(t *testing.T) {
	if NormalizeTitle("Amélie") != NormalizeTitle("Amélie") {
		t.Error("composed and decomposed Amélie normalise differently")
	}
}

func TestScore(t *testing.T) {
	tests := []struct {
		name      string
		query     Query
		candidate Candidate
		want      float64
	}{
		{"exact with year", Query{Title: "The Matrix", Year: 1999}, Candidate{Title: "The Matrix", Year: 1999}, 1},
		{"article and case ignored", Query{Title: "matrix", Year: 1999}, Candidate{Title: "The Matrix", Year: 1999}, 1},
		{"unknown query year", Query{Title: "The Matrix"}, Candidate{Title: "The Matrix", Year: 1999}, 0.9},
		{"unknown candidate year", Query{Title: "The Matrix", Year: 1999}, Candidate{Title: "The Matrix"}, 0.9},
		{"year off by one", Query{Title: "The Matrix", Year: 1999}, Candidate{Title: "The Matrix", Year: 2000}, 0.92},
		{"year off by one earlier", Query{Title: "The Matrix", Year: 1999}, Candidate{Title: "The Matrix", Year: 1998}, 0.92},
		{"year far", Query{Title: "The Matrix", Year: 1999}, Candidate{Title: "The Matrix", Year: 2010}, 0.6},
		{"original title wins", Query{Title: "Das Boot", Year: 1981}, Candidate{Title: "The Boat", OriginalTitle: "Das Boot", Year: 1981}, 1},
		{"one edit", Query{Title: "abcd", Year: 2000}, Candidate{Title: "abce", Year: 2000}, 0.75},
		{"empty query", Query{Year: 1999}, Candidate{Title: "The Matrix", Year: 1999}, 0},
		{"empty candidate", Query{Title: "Alien", Year: 1999}, Candidate{Year: 1999}, 0},
		{"only punctuation", Query{Title: "...", Year: 1999}, Candidate{Title: "The Matrix", Year: 1999}, 0},
		{"umlaut", Query{Title: "Müller", Year: 2001}, Candidate{Title: "Müller", Year: 2001}, 1},
		{"umlaut dropped is not equal", Query{Title: "Muller", Year: 2001}, Candidate{Title: "Müller", Year: 2001}, 5.0 / 6},
		{"decomposed equals composed", Query{Title: "Amélie", Year: 2001}, Candidate{Title: "Amélie", Year: 2001}, 1},
		{"cjk exact", Query{Title: "千と千尋の神隠し", Year: 2001}, Candidate{Title: "千と千尋の神隠し", Year: 2001}, 1},
		{"cjk original title", Query{Title: "千と千尋の神隠し", Year: 2001}, Candidate{Title: "Spirited Away", OriginalTitle: "千と千尋の神隠し", Year: 2001}, 1},
		{"exact id ignores title and year", Query{Title: "x", Year: 1, Ref: tmdbMovie("603")}, Candidate{Ref: tmdbMovie("603"), Title: "The Matrix", Year: 1999}, 1},
		{"other id gets title score", Query{Title: "The Matrix", Year: 1999, Ref: tmdbMovie("1")}, Candidate{Ref: tmdbMovie("603"), Title: "The Matrix", Year: 1999}, 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Score(test.query, test.candidate); math.Abs(got-test.want) > epsilon {
				t.Errorf("Score = %v, want %v", got, test.want)
			}
		})
	}
}

func TestScoreStaysBelowAutoMatch(t *testing.T) {
	tests := []struct {
		name      string
		query     Query
		candidate Candidate
	}{
		{"unrelated", Query{Title: "Alien", Year: 1979}, Candidate{Title: "Casablanca", Year: 1979}},
		{"sequel digit", Query{Title: "Toy Story 2", Year: 1999}, Candidate{Title: "Toy Story 3", Year: 1999}},
		{"toy story 3 vs 2", Query{Title: "Toy Story 3", Year: 2010}, Candidate{Title: "Toy Story 2", Year: 2010}},
		{"saw vs saw II", Query{Title: "Saw", Year: 2004}, Candidate{Title: "Saw II", Year: 2004}},
		{"saw II vs saw", Query{Title: "Saw II", Year: 2005}, Candidate{Title: "Saw", Year: 2005}},
		{"mission impossible 2 vs 3", Query{Title: "Mission Impossible 2", Year: 2000}, Candidate{Title: "Mission Impossible 3", Year: 2000}},
		{"digit vs roman", Query{Title: "Rocky 2", Year: 1979}, Candidate{Title: "Rocky III", Year: 1979}},
		{"number word", Query{Title: "Part Two", Year: 2000}, Candidate{Title: "Part Three", Year: 2000}},
		{"year in title", Query{Title: "Blade Runner", Year: 2017}, Candidate{Title: "Blade Runner 2049", Year: 2017}},
		{"subtitle only", Query{Title: "Star Wars", Year: 1977}, Candidate{Title: "Star Wars: A New Hope", Year: 1977}},
		{"subtitle on query", Query{Title: "Star Wars: A New Hope", Year: 1977}, Candidate{Title: "Star Wars", Year: 1977}},
		{"same main different subtitle", Query{Title: "Star Trek: Voyager", Year: 1995}, Candidate{Title: "Star Trek: Discovery", Year: 1995}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Score(test.query, test.candidate); got >= AutoMatchThreshold {
				t.Errorf("Score = %v, want < %v", got, AutoMatchThreshold)
			}
		})
	}
}

func TestScoreSameSequelNumberMatches(t *testing.T) {
	tests := []struct{ query, candidate string }{
		{"Toy Story 2", "Toy Story 2"},
		{"Toy Story 2", "Toy Story II"},
		{"Rocky 4", "Rocky IV"},
		{"Part Two", "Part 2"},
	}

	for _, test := range tests {
		got := Score(Query{Title: test.query, Year: 2000}, Candidate{Title: test.candidate, Year: 2000})
		if got < AutoMatchThreshold {
			t.Errorf("Score(%q, %q) = %v, want >= %v (numbers agree, so no cap)", test.query, test.candidate, got, AutoMatchThreshold)
		}
	}

	if numberSignature("rocky 007") != numberSignature("rocky 7") {
		t.Error("leading zeros must not change the number signature")
	}
}

func TestSubtitlePenaltyAndNumberCapStayBelowThreshold(t *testing.T) {
	if subtitlePenalty >= AutoMatchThreshold || numberMismatchCap >= AutoMatchThreshold {
		t.Error("penalty constants must stay below the auto-match threshold")
	}
}

func TestSimilarityCapsLongTitles(t *testing.T) {
	long := strings.Repeat("a", maxComparedRunes*50)
	other := strings.Repeat("a", maxComparedRunes*50) + "b"

	if got := similarity(long, other); math.Abs(got-1) > epsilon {
		t.Errorf("similarity of equal 200-rune prefixes = %v, want 1", got)
	}

	if got := similarity(long, strings.Repeat("b", maxComparedRunes*50)); got != 0 {
		t.Errorf("similarity of different long titles = %v, want 0", got)
	}
}

func TestEditDistanceKnownValues(t *testing.T) {
	tests := []struct {
		left, right string
		want        int
	}{
		{"kitten", "sitting", 3}, {"", "abc", 3}, {"abc", "", 3}, {"same", "same", 0}, {"flaw", "lawn", 2},
	}
	for _, test := range tests {
		if got := editDistance([]rune(test.left), []rune(test.right)); got != test.want {
			t.Errorf("editDistance(%q, %q) = %d, want %d", test.left, test.right, got, test.want)
		}
	}
}

func TestRankOrdersByConfidenceThenPopularityThenProviderOrder(t *testing.T) {
	candidates := []Candidate{
		{Ref: tmdbMovie("1"), Title: "Casablanca", Year: 1942, Popularity: 99},
		{Ref: tmdbMovie("2"), Title: "Alien", Year: 1979, Popularity: 5},
		{Ref: tmdbMovie("3"), Title: "Alien", Year: 1979, Popularity: 50},
		{Ref: tmdbMovie("4"), Title: "Alien", Year: 1979, Popularity: 5},
	}

	ranked := Rank(Query{Title: "Alien", Year: 1979}, candidates)

	for index, want := range []string{"3", "2", "4", "1"} {
		if got := ranked[index].Candidate.Ref.ID; got != want {
			t.Fatalf("position %d = %s, want %s", index, got, want)
		}
	}

	if candidates[0].Ref.ID != "1" {
		t.Error("Rank modified its input")
	}
}

func TestRankHandlesNilInput(t *testing.T) {
	if got := Rank(Query{Title: "x"}, nil); len(got) != 0 {
		t.Errorf("Rank(nil) = %v", got)
	}
}

func TestRankMarksExactID(t *testing.T) {
	ranked := Rank(Query{Title: "wrong", Ref: tmdbMovie("603")}, []Candidate{
		{Ref: tmdbMovie("1"), Title: "wrong"},
		{Ref: tmdbMovie("603"), Title: "The Matrix"},
		{Ref: ProviderRef{Provider: RefIMDb, ID: "603"}, Title: "other provider"},
	})

	if !ranked[0].ExactID || ranked[0].Candidate.Ref.ID != "603" || ranked[0].Confidence != 1 {
		t.Errorf("first = %+v, want exact ID 603 with confidence 1", ranked[0])
	}

	if ranked[1].ExactID || ranked[2].ExactID {
		t.Error("only the named entry is an exact ID match")
	}
}

func scored(confidences ...float64) []Scored {
	ranked := make([]Scored, 0, len(confidences))
	for _, confidence := range confidences {
		ranked = append(ranked, Scored{Candidate: Candidate{Ref: tmdbMovie("42")}, Confidence: confidence})
	}

	return ranked
}

func TestDecide(t *testing.T) {
	tests := []struct {
		name     string
		input    []Scored
		wantBest bool
		wantAuto bool
	}{
		{"nil", nil, false, false},
		{"empty", []Scored{}, false, false},
		{"below threshold", scored(0.84), true, false},
		{"at threshold single", scored(0.85), true, true},
		{"clear lead", scored(1, 0.7), true, true},
		{"lead boundary float", scored(0.95, 0.85), true, true},
		{"lead boundary low", scored(0.85, 0.75), true, true},
		{"just short of lead", scored(1, 0.91), true, false},
		{"no clear lead", scored(1, 0.95), true, false},
		{"two perfect", scored(1, 1), true, false},
		{"unsorted input", scored(0.5, 1, 0.7), true, true},
		{"exact id beats everything", []Scored{{Confidence: 1, ExactID: true}, {Confidence: 1}}, true, true},
		{"exact id with low score elsewhere", []Scored{{Confidence: 0.99}, {Confidence: 1, ExactID: true}}, true, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			best, auto := Decide(test.input)
			if (best != nil) != test.wantBest || auto != test.wantAuto {
				t.Errorf("Decide = %v, %v; want best=%v auto=%v", best, auto, test.wantBest, test.wantAuto)
			}
		})
	}
}

func TestDecideReturnsTheBestCandidate(t *testing.T) {
	input := []Scored{
		{Candidate: Candidate{Title: "low"}, Confidence: 0.3},
		{Candidate: Candidate{Title: "high"}, Confidence: 0.99},
	}

	best, auto := Decide(input)
	if !auto || best.Candidate.Title != "high" {
		t.Errorf("Decide = %+v, %v", best, auto)
	}
}

func TestResolve(t *testing.T) {
	confident := scored(1)
	weak := scored(0.5)

	tests := []struct {
		name       string
		current    MatchState
		locked     bool
		input      []Scored
		wantState  MatchState
		wantChosen bool
	}{
		{"disabled stays disabled", MatchStateDisabled, false, confident, MatchStateDisabled, false},
		{"locked unmatched stays", MatchStateUnmatched, true, confident, MatchStateUnmatched, false},
		{"locked matched stays", MatchStateMatched, true, confident, MatchStateMatched, false},
		{"unmatched gets match", MatchStateUnmatched, false, confident, MatchStateMatched, true},
		{"matched refreshed", MatchStateMatched, false, confident, MatchStateMatched, true},
		{"unmatched weak stays unmatched", MatchStateUnmatched, false, weak, MatchStateUnmatched, false},
		{"unmatched empty stays unmatched", MatchStateUnmatched, false, nil, MatchStateUnmatched, false},
		{"matched empty lookup keeps match", MatchStateMatched, false, nil, MatchStateMatched, false},
		{"matched weak lookup keeps match", MatchStateMatched, false, weak, MatchStateMatched, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := Resolve(test.current, test.locked, test.input)
			if decision.State != test.wantState || (decision.Chosen != nil) != test.wantChosen {
				t.Errorf("Resolve = %+v, want state %q chosen %v", decision, test.wantState, test.wantChosen)
			}
		})
	}
}

func TestEndToEndAutoMatchAndAmbiguity(t *testing.T) {
	blade := Rank(Query{Title: "Blade Runner", Year: 1982}, []Candidate{
		{Ref: tmdbMovie("78"), Title: "Blade Runner", Year: 1982},
		{Ref: tmdbMovie("335984"), Title: "Blade Runner 2049", Year: 2017},
	})
	if best, auto := Decide(blade); !auto || best.Candidate.Ref.ID != "78" {
		t.Errorf("Blade Runner = %+v, %v", best, auto)
	}

	dune := []Candidate{
		{Ref: tmdbMovie("841"), Title: "Dune", Year: 1984},
		{Ref: tmdbMovie("438631"), Title: "Dune", Year: 2021},
	}

	if _, auto := Decide(Rank(Query{Title: "Dune"}, dune)); auto {
		t.Error("Dune without a year must not auto-match")
	}

	best, auto := Decide(Rank(Query{Title: "Dune", Year: 1984}, dune))
	if !auto || best.Candidate.Ref.ID != "841" {
		t.Errorf("Dune 1984 = %+v, %v", best, auto)
	}

	best, auto = Decide(Rank(Query{Title: "Dune", Year: 2021}, dune))
	if !auto || best.Candidate.Ref.ID != "438631" {
		t.Errorf("Dune 2021 = %+v, %v", best, auto)
	}
}

func TestEndToEndSequelsNeverAutoMatchEachOther(t *testing.T) {
	tests := []struct {
		query Query
		other Candidate
	}{
		{Query{Title: "Toy Story 2", Year: 1999}, Candidate{Ref: tmdbMovie("863"), Title: "Toy Story 3", Year: 1999}},
		{Query{Title: "Saw", Year: 2004}, Candidate{Ref: tmdbMovie("215"), Title: "Saw II", Year: 2004}},
		{Query{Title: "Mission Impossible 2", Year: 2000}, Candidate{Ref: tmdbMovie("955"), Title: "Mission: Impossible 3", Year: 2000}},
	}

	for _, test := range tests {
		if _, auto := Decide(Rank(test.query, []Candidate{test.other})); auto {
			t.Errorf("%q auto-matched %q", test.query.Title, test.other.Title)
		}
	}
}

func TestParseProviderRef(t *testing.T) {
	const uuid = "b10bbbfc-cf9e-42e0-be17-e2c3e1d2600d"

	tests := []struct {
		name string
		in   string
		want ProviderRef
	}{
		{"bare tmdb", "603", ProviderRef{Provider: RefTMDB, ID: "603"}},
		{"bare tmdb spaces", "  603 \n", ProviderRef{Provider: RefTMDB, ID: "603"}},
		{"bare imdb", "tt0133093", ProviderRef{Provider: RefIMDb, ID: "tt0133093"}},
		{"bare uuid", uuid, ProviderRef{Provider: RefMusicBrainz, ID: uuid}},
		{"bare uuid upper", strings.ToUpper(uuid), ProviderRef{Provider: RefMusicBrainz, ID: uuid}},
		{"tmdb movie url", "https://www.themoviedb.org/movie/603-the-matrix", tmdbMovie("603")},
		{"tmdb movie url no slug", "https://themoviedb.org/movie/603", tmdbMovie("603")},
		{"tmdb movie url language", "https://www.themoviedb.org/movie/603-the-matrix?language=de-DE", tmdbMovie("603")},
		{"tmdb tv url", "http://themoviedb.org/tv/1396-breaking-bad/season/1", ProviderRef{Provider: RefTMDB, Kind: RefKindShow, ID: "1396"}},
		{"tmdb url without scheme", "themoviedb.org/movie/603", tmdbMovie("603")},
		{"imdb url", "https://www.imdb.com/title/tt0133093/", ProviderRef{Provider: RefIMDb, ID: "tt0133093"}},
		{"imdb mobile url", "https://m.imdb.com/title/tt0133093/?ref_=x", ProviderRef{Provider: RefIMDb, ID: "tt0133093"}},
		{"musicbrainz release", "https://musicbrainz.org/release/" + uuid, ProviderRef{Provider: RefMusicBrainz, Kind: RefKindRelease, ID: uuid}},
		{"musicbrainz artist", "https://musicbrainz.org/artist/" + uuid + "/releases", ProviderRef{Provider: RefMusicBrainz, Kind: RefKindArtist, ID: uuid}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseProviderRef(test.in)
			if err != nil || got != test.want {
				t.Errorf("ParseProviderRef(%q) = %+v, %v; want %+v", test.in, got, err, test.want)
			}
		})
	}
}

func TestParseProviderRefRejects(t *testing.T) {
	const uuid = "b10bbbfc-cf9e-42e0-be17-e2c3e1d2600d"

	for _, text := range []string{
		"", "   ", "hello", "0", "0603", "-5", "tt123", "TT0133093",
		"ftp://themoviedb.org/movie/603",
		"https://example.com/movie/603",
		"https://evil.example/https://themoviedb.org/movie/603",
		"https://www.themoviedb.org/person/603",
		"https://www.themoviedb.org/movie/abc",
		"https://www.themoviedb.org/movie",
		"https://www.themoviedb.org/movie/0603",
		"https://www.imdb.com/name/nm0000206/",
		"https://www.imdb.com/title/tt12/",
		"https://musicbrainz.org/recording/" + uuid,
		"https://musicbrainz.org/release/not-a-uuid",
		"https://musicbrainz.org/release/b10bbbfc-cf9e-42e0-be17-e2c3e1d2600",
		"https://musicbrainz.org/release/g10bbbfc-cf9e-42e0-be17-e2c3e1d2600d",
		"https://musicbrainz.org/release/b10bbbfc_cf9e-42e0-be17-e2c3e1d2600d",
		"http://[::1",
		"https://themoviedb.org.evil.example/movie/603",
	} {
		if _, err := ParseProviderRef(text); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("ParseProviderRef(%q) error = %v, want ErrInvalid", text, err)
		}
	}
}

func TestProviderRefHelpers(t *testing.T) {
	if !(ProviderRef{}).IsZero() || tmdbMovie("1").IsZero() {
		t.Error("IsZero wrong")
	}

	tests := []struct {
		ref  ProviderRef
		want metadata.ProviderID
		ok   bool
	}{
		{tmdbMovie("603"), metadata.ProviderID{Provider: metadata.ProviderTMDB, Value: "603"}, true},
		{ProviderRef{Provider: RefIMDb, ID: "tt0133093"}, metadata.ProviderID{Provider: metadata.ProviderIMDb, Value: "tt0133093"}, true},
		{ProviderRef{Provider: RefMusicBrainz, ID: "x"}, metadata.ProviderID{}, false},
	}

	for _, test := range tests {
		got, ok := test.ref.ProviderID()
		if got != test.want || ok != test.ok {
			t.Errorf("%+v.ProviderID() = %+v, %v", test.ref, got, ok)
		}
	}
}
