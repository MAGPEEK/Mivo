package naming

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/magpeek/mivo/internal/domain/library"
)

// defaultSeason is the season of an episode that names none (ADR 0027 §4).
const defaultSeason = 1

// specialsSeason is the season number of specials; its title and the other seasons' title
// prefix are display data.
const (
	specialsSeason    = 0
	specialsTitle     = "Specials"
	seasonTitlePrefix = "Season "
)

// specialsFolderName is the season-0 folder (compared case-folded).
const specialsFolderName = "specials"

// seasonFolderPattern matches "Season 1", "Season 01", "Staffel 2", "S2".
// A trailing bracket or parenthesis group is ignored ("Season 2 [1080p]", "Season 1 (2020)").
var seasonFolderPattern = regexp.MustCompile(`(?i)^(?:season|staffel|s)[\s._-]*(\d{1,4})(?:[\s._-]*[\[(].*)?$`)

// seasonFolderNumber returns the season a folder name stands for.
func seasonFolderNumber(name string) (int, bool) {
	trimmed := strings.TrimSpace(name)
	if strings.EqualFold(trimmed, specialsFolderName) {
		return specialsSeason, true
	}

	match := seasonFolderPattern.FindStringSubmatch(trimmed)
	if match == nil {
		return 0, false
	}

	return parseDigits(match[1]), true
}

// seasonOfFolders is the season of the deepest season folder among the directories below the
// show folder; other folder names do not set a season.
func seasonOfFolders(directories []string) (int, bool) {
	for index := len(directories) - 1; index >= 0; index-- {
		if number, ok := seasonFolderNumber(directories[index]); ok {
			return number, true
		}
	}

	return 0, false
}

// seasonLevel is the season item; its key is the season number.
func seasonLevel(number int) library.Level {
	title := seasonTitlePrefix + strconv.Itoa(number)
	if number == specialsSeason {
		title = specialsTitle
	}

	return library.Level{
		Kind:     library.ItemKindSeason,
		MatchKey: strconv.Itoa(number),
		Title:    title,
		Number:   library.NumberOf(number),
	}
}

// parseDigits converts a run of ASCII digits that a pattern already validated.
func parseDigits(digits string) int {
	value, err := strconv.Atoi(digits)
	if err != nil {
		return 0
	}

	return value
}
