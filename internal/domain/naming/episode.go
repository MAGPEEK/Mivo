package naming

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/magpeek/mivo/internal/domain/library"
)

// Patterns of episode markers, matched against single lower-case tokens.
var (
	seasonEpisodePattern = regexp.MustCompile(`^s(\d{1,4})e(\d{1,4})((?:-?e\d{1,4}|-\d{1,4})*)$`)
	crossPattern         = regexp.MustCompile(`^(\d{1,2})x(\d{1,4})(?:-(?:\d{1,2}x)?(\d{1,4}))?$`)
	episodeOnlyPattern   = regexp.MustCompile(`^(?:e|ep)(\d{1,4})(?:-(?:e|ep)?(\d{1,4}))?$`)
	episodeRangePattern  = regexp.MustCompile(`^(\d{1,4})(?:-(\d{1,4}))?$`)
	digitRunPattern      = regexp.MustCompile(`\d+`)
)

// episodeWord introduces a spelled-out marker ("Episode 2").
const episodeWord = "episode"

// episodeTitlePrefix starts the placeholder title of an episode whose name has no title.
const episodeTitlePrefix = "Episode "

// spelledWidth is the token count of "Episode 2".
const spelledWidth = 2

// versionSeparator joins several version labels of one episode name.
const versionSeparator = " - "

// episodeMarker is the episode number a file name carries. index and width locate its tokens.
// season is absent when the marker names none; absolute marks bare numbers ("Show - 130"),
// which are also the absolute episode number. Explicit E02/Episode 2 markers are not absolute:
// inside a season folder they count within the season.
type episodeMarker struct {
	index    int
	width    int
	season   library.OptionalNumber
	number   int
	end      int
	absolute bool
}

// findMarker looks for an episode marker among the title tokens (those before the first
// release tag). Explicit markers win over bare numbers; bare numbers may start at token
// firstBare (0 inside a show folder, 1 for root files that need a title first).
func findMarker(tokens []string, firstBare int) (episodeMarker, bool) {
	title := tokens[:firstReleaseTag(tokens)]

	for index := range title {
		if marker, ok := parseExplicitMarker(title, index); ok {
			return marker, true
		}
	}

	return findAbsolute(title, firstBare)
}

func parseExplicitMarker(tokens []string, index int) (episodeMarker, bool) {
	lower := strings.ToLower(tokens[index])

	if match := seasonEpisodePattern.FindStringSubmatch(lower); match != nil {
		return seasonEpisodeMarker(index, match), true
	}

	if match := crossPattern.FindStringSubmatch(lower); match != nil {
		return newMarker(index, library.NumberOf(parseDigits(match[1])), match[2], match[3]), true
	}

	if match := episodeOnlyPattern.FindStringSubmatch(lower); match != nil {
		return newMarker(index, library.OptionalNumber{}, match[1], match[2]), true
	}

	if lower == episodeWord && index+1 < len(tokens) {
		if match := episodeRangePattern.FindStringSubmatch(tokens[index+1]); match != nil {
			marker := newMarker(index, library.OptionalNumber{}, match[1], match[2])
			marker.width = spelledWidth

			return marker, true
		}
	}

	return episodeMarker{}, false
}

func seasonEpisodeMarker(index int, match []string) episodeMarker {
	end := match[3]
	if runs := digitRunPattern.FindAllString(end, -1); len(runs) > 0 {
		end = runs[len(runs)-1]
	}

	return newMarker(index, library.NumberOf(parseDigits(match[1])), match[2], end)
}

// newMarker builds a marker; an end before the start is dropped (it was no range).
func newMarker(index int, season library.OptionalNumber, start, end string) episodeMarker {
	marker := episodeMarker{index: index, width: 1, season: season, number: parseDigits(start)}

	if end != "" && parseDigits(end) >= marker.number {
		marker.end = parseDigits(end)
	}

	return marker
}

// episodeKey is the match key of a numbered episode: "<n>" or "<n>-<end>".
func (marker episodeMarker) episodeKey() string {
	key := strconv.Itoa(marker.number)
	if marker.end > marker.number {
		key += "-" + strconv.Itoa(marker.end)
	}

	return key
}

// episodeLevel builds the episode item for a marker and its title.
func episodeLevel(marker episodeMarker, title string) library.Level {
	level := library.Level{
		Kind:     library.ItemKindEpisode,
		MatchKey: marker.episodeKey(),
		Title:    title,
		Number:   library.NumberOf(marker.number),
	}

	if marker.end > marker.number {
		level.NumberEnd = library.NumberOf(marker.end)
	}

	if marker.absolute {
		level.AbsoluteNumber = library.NumberOf(marker.number)
	}

	return level
}

// splitEpisodeName splits the words after the marker at dashes: the first part is the episode
// title, further parts are the version label ("S01E01 - Pilot - 4K" gives "Pilot" and "4K").
// Without words the title is "Episode N".
func splitEpisodeName(words []string, number int) (title, label string) {
	var parts, current []string

	flush := func() {
		if len(current) > 0 {
			parts = append(parts, strings.Join(current, " "))
			current = nil
		}
	}

	for _, word := range words {
		if word == dashToken {
			flush()

			continue
		}

		current = append(current, word)
	}

	flush()

	if len(parts) == 0 {
		return episodeTitlePrefix + strconv.Itoa(number), ""
	}

	return parts[0], strings.Join(parts[1:], versionSeparator)
}

// wordsAfterMarker returns the title tokens after the marker, up to the first release tag.
func wordsAfterMarker(tokens []string, marker episodeMarker) []string {
	title := tokens[:firstReleaseTag(tokens)]

	return title[marker.index+marker.width:]
}
