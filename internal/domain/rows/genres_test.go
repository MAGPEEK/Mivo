package rows_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/magpeek/mivo/internal/domain/rows"
)

// twelveGenres has distinct counts: Genre01 has the most items, Genre12 the fewest.
func twelveGenres() []rows.GenreCount {
	counts := make([]rows.GenreCount, 0, 12)
	for index := 1; index <= 12; index++ {
		counts = append(counts, rows.GenreCount{Genre: fmt.Sprintf("Genre%02d", index), Count: int64(100 - index)})
	}

	return counts
}

func TestPickGenresIsDeterministicPerDay(t *testing.T) {
	t.Parallel()

	const today, tomorrow = 20261105, 20261106

	first := rows.PickGenres(twelveGenres(), today)
	shuffled := twelveGenres()
	slices.Reverse(shuffled)

	if again := rows.PickGenres(shuffled, today); !slices.Equal(first, again) {
		t.Errorf("same day picked %v, then %v", first, again)
	}

	if len(first) != rows.MaxGenreRows {
		t.Fatalf("picked %d genres, want %d", len(first), rows.MaxGenreRows)
	}

	for _, genre := range first {
		if genre == "Genre11" || genre == "Genre12" {
			t.Errorf("picked %q outside the top ten", genre)
		}
	}

	changed := false

	for seed := int64(tomorrow); seed < tomorrow+30 && !changed; seed++ {
		changed = !slices.Equal(first, rows.PickGenres(twelveGenres(), seed))
	}

	if !changed {
		t.Error("the pick never rotates across 30 days")
	}
}

func TestPickGenresEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		counts []rows.GenreCount
		want   []string
	}{
		{name: "no genres", counts: nil, want: []string{}},
		{name: "empty names and counts are ignored", counts: []rows.GenreCount{{Genre: " ", Count: 5}, {Genre: "Drama", Count: 0}}, want: []string{}},
		{name: "fewer than three", counts: []rows.GenreCount{{Genre: "Drama", Count: 2}}, want: []string{"Drama"}},
		{name: "case variants keep the larger", counts: []rows.GenreCount{{Genre: "drama", Count: 1}, {Genre: "Drama", Count: 4}}, want: []string{"Drama"}},
		{name: "names too long for the API", counts: []rows.GenreCount{{Genre: strings.Repeat("g", rows.MaxGenreRunes+1), Count: 9}}, want: []string{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := rows.PickGenres(test.counts, 1); !slices.Equal(got, test.want) {
				t.Errorf("PickGenres = %v, want %v", got, test.want)
			}
		})
	}
}

func TestPickGenresRanksTiesByName(t *testing.T) {
	t.Parallel()

	// Eleven genres with the same count: the top ten by name leave out "k".
	counts := make([]rows.GenreCount, 0, 11)
	for _, name := range strings.Split("k j i h g f e d c b a", " ") {
		counts = append(counts, rows.GenreCount{Genre: name, Count: 1})
	}

	for seed := range int64(50) {
		if slices.Contains(rows.PickGenres(counts, seed), "k") {
			t.Fatalf("seed %d picked the eleventh genre", seed)
		}
	}
}

func TestPickGenresSumsCaseVariants(t *testing.T) {
	t.Parallel()

	// Apart, "Drama" (4) and "drama" (3) would rank behind the ten genres with 5 items;
	// together (7) they rank first and push "j" out of the top ten.
	counts := []rows.GenreCount{{Genre: "Drama", Count: 4}, {Genre: "drama", Count: 3}}
	for _, name := range strings.Split("a b c d e f g h i j", " ") {
		counts = append(counts, rows.GenreCount{Genre: name, Count: 5})
	}

	pickedDrama := false

	for seed := range int64(50) {
		picked := rows.PickGenres(counts, seed)
		if slices.Contains(picked, "j") || slices.Contains(picked, "drama") {
			t.Fatalf("seed %d picked %v, want the merged Drama in the top ten instead of j", seed, picked)
		}

		pickedDrama = pickedDrama || slices.Contains(picked, "Drama")
	}

	if !pickedDrama {
		t.Error("the merged Drama was never picked")
	}
}

func TestSummaryLine(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("word ", 60)
	tests := []struct {
		name    string
		summary string
		want    string
	}{
		{name: "empty", summary: "", want: ""},
		{name: "short with line breaks", summary: "  A rabbit.\n\nThree bullies. ", want: "A rabbit. Three bullies."},
		{name: "long text cut at a word", summary: long, want: strings.TrimSpace(strings.Repeat("word ", 31)) + "…"},
		{name: "one long word", summary: strings.Repeat("x", 200), want: strings.Repeat("x", rows.MaxSummaryLineRunes-1) + "…"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := rows.SummaryLine(test.summary)
			if got != test.want {
				t.Errorf("SummaryLine = %q, want %q", got, test.want)
			}

			if utf8.RuneCountInString(got) > rows.MaxSummaryLineRunes {
				t.Errorf("SummaryLine has %d runes, more than %d", utf8.RuneCountInString(got), rows.MaxSummaryLineRunes)
			}
		})
	}
}
