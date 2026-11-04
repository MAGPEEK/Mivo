package naming

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/magpeek/mivo/internal/domain/library"
)

// labelEdgeCharacters are trimmed from both ends of a version label: separators and brackets.
const labelEdgeCharacters = " ._-()[]{}"

// editionPattern matches the Plex edition marker "{edition-Director's Cut}".
var editionPattern = regexp.MustCompile(`(?i)\{edition-([^}]*)\}`)

// partPattern matches a part marker with a separator before it and no separator between the
// word and the number ("- cd1", ".part2"), so the title "Part 1" is not a part. The trailing
// group keeps the character after the number so the cut does not join two words.
var partPattern = regexp.MustCompile(`(?i)[\s._-](?:cd|disc|disk|part|pt)([1-9])($|[^\p{L}\p{N}])`)

// spacedPartPattern accepts one separator between word and number ("Movie - cd 1",
// "Movie.Part.2") only at the very end of the name, so a title such as "Deathly Hallows Part 1
// (2010)", where the year follows, stays a title.
var spacedPartPattern = regexp.MustCompile(`(?i)[\s._-](?:cd|disc|disk|part|pt)[\s._-]([1-9])(\s*)$`)

// versionMarkers are the edition and part a file name carries besides its title.
type versionMarkers struct {
	rest    string
	edition string
	part    library.OptionalNumber
}

// extractVersionMarkers cuts the edition marker and the part marker out of a file stem.
func extractVersionMarkers(stem string) versionMarkers {
	markers := versionMarkers{rest: stem}

	if match := editionPattern.FindStringSubmatchIndex(markers.rest); match != nil {
		markers.edition = strings.TrimSpace(markers.rest[match[2]:match[3]])
		markers.rest = markers.rest[:match[0]] + " " + markers.rest[match[1]:]
	}

	for _, pattern := range []*regexp.Regexp{partPattern, spacedPartPattern} {
		if markers.cutPart(pattern) {
			break
		}
	}

	return markers
}

// cutPart removes the first part marker the pattern finds (group 1: number, group 2: the
// kept tail) and records its number.
func (markers *versionMarkers) cutPart(pattern *regexp.Regexp) bool {
	match := pattern.FindStringSubmatchIndex(markers.rest)
	if match == nil {
		return false
	}

	// The pattern captures exactly one ASCII digit 1–9.
	markers.part = library.NumberOf(int(markers.rest[match[2]] - '0'))
	markers.rest = markers.rest[:match[0]] + markers.rest[match[4]:]

	return true
}

// versionLabel joins the cleaned remainder of a file name and its edition.
func versionLabel(remainder, edition string) string {
	label := strings.Trim(remainder, labelEdgeCharacters)

	switch {
	case label == "":
		return edition
	case edition == "":
		return label
	default:
		return label + " " + edition
	}
}

// cutNamePrefix returns what follows the prefix of name whose normalised form equals key, if
// the prefix ends at a word boundary ("Movie (2010) - 4K" has the prefix "Movie (2010)" for
// the key "movie 2010", "Movies 2" has none for "movie").
func cutNamePrefix(name, key string) (string, bool) {
	if key == "" {
		return "", false
	}

	for index := range name {
		normalised := library.NormalizeKey(name[:index])
		if normalised == key && atWordBoundary(name, index) {
			return name[index:], true
		}

		if !strings.HasPrefix(key, normalised) {
			return "", false
		}
	}

	if library.NormalizeKey(name) == key {
		return "", true
	}

	return "", false
}

// atWordBoundary reports whether index (inside name, after at least one character) does not
// split a word.
func atWordBoundary(name string, index int) bool {
	next, _ := utf8.DecodeRuneInString(name[index:])
	previous, _ := utf8.DecodeLastRuneInString(name[:index])

	return !library.IsKeyCharacter(next) || !library.IsKeyCharacter(previous)
}
