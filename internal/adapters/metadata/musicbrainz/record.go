package musicbrainz

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/magpeek/mivo/internal/domain/metadata"
)

const (
	// maxGenres bounds the genres taken from the votes; long tails are noise.
	maxGenres = 5

	areaTypeCountry = "Country"

	yearDigits     = 4
	fullDateLength = len("2006-01-02")
)

// Records carry no provider IDs and no people: MusicBrainz IDs are not a metadata.Provider
// (they travel in the match reference) and the record has no artist role, so artist
// credits are only used to search.

func releaseGroupRecord(group releaseGroup) metadata.Record {
	record := metadata.Record{Title: group.Title, Genres: genres(group.Genres, group.Tags)}
	record.Year, record.ReleaseDate = parsePartialDate(group.FirstReleaseDate)

	return record
}

// releaseRecord maps one edition. Year and date come from its release group when known,
// because an album is listed under its first release, not under the reissue in hand.
func releaseRecord(edition release) metadata.Record {
	record := metadata.Record{
		Title:   edition.Title,
		Genres:  genres(edition.Genres, edition.Tags),
		Studios: labelNames(edition.LabelInfo),
	}
	record.Year, record.ReleaseDate = parsePartialDate(edition.Date)

	if edition.ReleaseGroup != nil {
		if year, date := parsePartialDate(edition.ReleaseGroup.FirstReleaseDate); year > 0 {
			record.Year, record.ReleaseDate = year, date
		}
	}

	return record
}

func artistRecord(entry artist) metadata.Record {
	record := metadata.Record{Title: entry.Name, SortTitle: entry.SortName, Genres: genres(entry.Genres, entry.Tags)}

	if entry.Area != nil && entry.Area.Type == areaTypeCountry && entry.Area.Name != "" {
		record.Countries = []string{entry.Area.Name}
	}

	return record
}

// genres prefers the curated genre votes and falls back to free tags; names are ordered by
// votes and case-insensitive duplicates dropped.
func genres(genreVotes, tagVotes []vote) []string {
	votes := genreVotes
	if len(votes) == 0 {
		votes = tagVotes
	}

	sorted := slices.Clone(votes)
	slices.SortStableFunc(sorted, func(left, right vote) int { return cmp.Compare(right.Count, left.Count) })

	var names []string

	seen := map[string]bool{}

	for _, entry := range sorted {
		name := strings.TrimSpace(entry.Name)
		key := strings.ToLower(name)

		if name == "" || seen[key] {
			continue
		}

		seen[key] = true

		if names = append(names, name); len(names) == maxGenres {
			break
		}
	}

	return names
}

func labelNames(infos []labelInfo) []string {
	var names []string

	for _, info := range infos {
		if info.Label != nil && info.Label.Name != "" && !slices.Contains(names, info.Label.Name) {
			names = append(names, info.Label.Name)
		}
	}

	return names
}

// parsePartialDate reads MusicBrainz dates, which may be "YYYY", "YYYY-MM" or "YYYY-MM-DD":
// the year from any of them, the date only from the full form.
func parsePartialDate(text string) (int, metadata.Date) {
	if len(text) < yearDigits {
		return 0, metadata.Date{}
	}

	year, err := strconv.Atoi(text[:yearDigits])
	if err != nil || year <= 0 {
		return 0, metadata.Date{}
	}

	if len(text) != fullDateLength {
		return year, metadata.Date{}
	}

	date, err := metadata.ParseDate(text)
	if err != nil {
		return year, metadata.Date{}
	}

	return year, date
}
