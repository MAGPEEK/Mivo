package naming

import (
	"regexp"
	"slices"
)

// bareNumberPattern matches a 1–4 digit episode number with an optional "v2" revision suffix
// ("05v2"): the revision is a re-release of the same episode, not a number.
var bareNumberPattern = regexp.MustCompile(`^(\d{1,4})(?:v\d+)?$`)

// dashToken separates title parts in release names ("Show - 05 - Title").
const dashToken = "-"

// Ranks of bare-number candidates; the lowest rank wins, the first candidate of a rank.
const (
	rankDashSeparated = iota // "Show - 05", "Show - 05 - Title"
	rankDashPreceded         // "Show - 05 Title"
	rankPlain                // "Show 05"
)

// resolutionHeights are the video heights that are not episode numbers when written without
// the "p" ("Show 05 - 1080"), unless no other number exists.
//
//nolint:gochecknoglobals // read-only lookup table, built once like the compiled patterns
var resolutionHeights = []int{480, 576, 720, 1080, 1440, 2160, 4320}

// findAbsolute finds a bare episode number among the title tokens from index first on
// (absolute numbering, UX DT-24). Years are never episode numbers; a number set off by dashes
// ("Show 2 - 05", "Show - 3 Days - 05") wins over one inside the title; a bare resolution
// height is only a last resort. The number is never split into season and episode: 105 is
// episode 105.
func findAbsolute(tokens []string, first int) (episodeMarker, bool) {
	var candidates []int

	for index := first; index < len(tokens); index++ {
		if isBareEpisodeNumber(tokens[index]) {
			candidates = append(candidates, index)
		}
	}

	if len(candidates) == 0 {
		return episodeMarker{}, false
	}

	candidates = withoutResolutions(tokens, candidates)
	chosen := candidates[0]

	for _, index := range candidates {
		if rankOf(tokens, index) < rankOf(tokens, chosen) {
			chosen = index
		}
	}

	number := bareNumberOf(tokens[chosen])

	return episodeMarker{index: chosen, width: 1, number: number, absolute: true}, true
}

// rankOf scores how much a candidate looks like an episode number slot.
func rankOf(tokens []string, index int) int {
	precededByDash := index > 0 && tokens[index-1] == dashToken
	endsPart := index+1 == len(tokens) || tokens[index+1] == dashToken

	switch {
	case precededByDash && endsPart:
		return rankDashSeparated
	case precededByDash:
		return rankDashPreceded
	default:
		return rankPlain
	}
}

// withoutResolutions drops resolution heights unless they are all there is.
func withoutResolutions(tokens []string, candidates []int) []int {
	kept := make([]int, 0, len(candidates))

	for _, index := range candidates {
		if !slices.Contains(resolutionHeights, bareNumberOf(tokens[index])) {
			kept = append(kept, index)
		}
	}

	if len(kept) == 0 {
		return candidates
	}

	return kept
}

func bareNumberOf(token string) int {
	return parseDigits(bareNumberPattern.FindStringSubmatch(token)[1])
}

func isBareEpisodeNumber(token string) bool {
	if !bareNumberPattern.MatchString(token) {
		return false
	}

	_, _, isYear := YearOf(token)

	return !isYear
}
