package rows

import (
	"cmp"
	"hash/fnv"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Bounds of the "moreGenre" rotation (API Row): up to MaxGenreRows rows picked from the
// RankedGenres genres with the most items.
const (
	RankedGenres = 10
	MaxGenreRows = 3
)

// MaxGenreRunes is the longest genre a row can carry (API Row.genre, Row.titleParams values
// and BrowseQuery.genre items: at most 200 characters).
const MaxGenreRunes = 200

// Factors that compose the date seed YYYYMMDD.
const (
	yearFactor  = 10000
	monthFactor = 100
)

// DateSeed returns the seed of the "moreGenre" rotation for the calendar date of t in t's own
// location, as the number YYYYMMDD (API Row): every request of one day picks the same genres.
func DateSeed(t time.Time) int64 {
	year, month, day := t.Date()

	return int64(year*yearFactor + int(month)*monthFactor + day)
}

// GenreCount is a genre with the number of items that carry it.
type GenreCount struct {
	Genre string
	Count int64
}

// PickGenres returns the genres of the "moreGenre" rows for the seed: the genres are ranked
// by count (most first, ties by case-folded name), and of the top RankedGenres up to
// MaxGenreRows are picked in an order derived from the seed alone. The pick is
// deterministic: the same counts and seed always yield the same genres in the same order.
// Genres without a name or items are ignored, and so are names longer than MaxGenreRunes,
// which the API cannot carry as a row genre or a filter value.
func PickGenres(counts []GenreCount, seed int64) []string {
	ranked := rankGenres(counts)

	// A hash of seed and name instead of math/rand: its order cannot change with a Go
	// release, so every server version picks the same genres on the same day.
	slices.SortFunc(ranked, func(left, right GenreCount) int {
		return cmp.Or(cmp.Compare(rotationKey(seed, left.Genre), rotationKey(seed, right.Genre)),
			strings.Compare(left.Genre, right.Genre))
	})

	picked := make([]string, 0, MaxGenreRows)
	for _, count := range ranked[:min(MaxGenreRows, len(ranked))] {
		picked = append(picked, count.Genre)
	}

	return picked
}

// rankGenres returns the top RankedGenres usable genres by count (ties by case-folded name).
// Spellings of one genre that differ only in case (the catalog compares genres
// case-insensitively, but the counts may still name both) become one genre with the sum of
// their counts, named by the spelling with the most items.
func rankGenres(counts []GenreCount) []GenreCount {
	usable := slices.DeleteFunc(slices.Clone(counts), func(count GenreCount) bool {
		return count.Count <= 0 || strings.TrimSpace(count.Genre) == "" || utf8.RuneCountInString(count.Genre) > MaxGenreRunes
	})

	sortByCount(usable)

	merged := make([]GenreCount, 0, len(usable))
	positions := make(map[string]int, len(usable))

	for _, count := range usable {
		folded := strings.ToLower(count.Genre)
		if position, seen := positions[folded]; seen {
			merged[position].Count += count.Count

			continue
		}

		positions[folded] = len(merged)
		merged = append(merged, count)
	}

	sortByCount(merged)

	return merged[:min(RankedGenres, len(merged))]
}

// sortByCount orders genres by count, most first, ties by case-folded name, then by name.
func sortByCount(counts []GenreCount) {
	slices.SortFunc(counts, func(left, right GenreCount) int {
		return cmp.Or(cmp.Compare(right.Count, left.Count),
			strings.Compare(strings.ToLower(left.Genre), strings.ToLower(right.Genre)),
			strings.Compare(left.Genre, right.Genre))
	})
}

// rotationKey is the place of a genre in the rotation of one seed.
func rotationKey(seed int64, genre string) uint64 {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(strconv.FormatInt(seed, 10) + "\x00" + strings.ToLower(genre))) // never fails

	return hash.Sum64()
}
