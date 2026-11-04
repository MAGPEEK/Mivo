package naming

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/magpeek/mivo/internal/domain/library"
)

// Year bounds of ADR 0027 §4: anything outside is part of the title ("Blade Runner 2049"
// still needs the last-year rule, see lastYear).
const (
	minimumYear = 1880
	maximumYear = 2100
)

// Resolution bounds for "<number>p" tags (480p … 4320p).
const (
	minimumResolution = 480
	maximumResolution = 4320
)

// releaseGroupSeparator joins a release tag and its group ("x264-GROUP").
const releaseGroupSeparator = "-"

// tagBrackets may wrap a release tag ("[1080p]").
const tagBrackets = "[]()"

// tokenPattern splits a name into tokens. "." and "_" separate words except inside the known
// dotted tags and abbreviations listed before the generic word alternative; Go's regexp
// prefers the earlier alternative at the same position.
var tokenPattern = regexp.MustCompile(`(?i)h\.26[45]|ddp?\d\.\d|[57]\.1|` +
	`(?:\p{L}\.){2,}|(?:mrs|mr|ms|dr|st|jr|sr|vol|vs)\.|[^\s._]+`)

// checksumPattern matches a bracketed CRC32 such as "[ABCD1234]".
var checksumPattern = regexp.MustCompile(`\[[0-9A-Fa-f]{8}\]`)

// leadingGroupPattern matches a release group in brackets at the start ("[Group] Show").
var leadingGroupPattern = regexp.MustCompile(`^\s*\[[^\]]*\]`)

var resolutionPattern = regexp.MustCompile(`^(\d{3,4})p$`)

var yearPattern = regexp.MustCompile(`^(?:\((\d{4})\)|\[(\d{4})\]|(\d{4}))$`)

// releaseTagPattern matches the lower-case tags that end a title, besides resolutions. The
// compiled pattern is the read-only lookup table: built once, safe for concurrent use.
var releaseTagPattern = regexp.MustCompile(`^(?:4k|uhd|x264|x265|h\.?264|h\.?265|hevc|avc|av1|` +
	`xvid|divx|10bit|bluray|bdrip|brrip|web-dl|webrip|hdtv|dvdrip|remux|aac|ac3|dts|truehd|` +
	`atmos|ddp5\.1|5\.1|7\.1|hdr|hdr10|dv)$`)

// TitleYear is the cleaned title of a name and its year, if it has one.
type TitleYear struct {
	Title string
	Year  library.OptionalNumber
}

// Tokenize removes bracketed CRC32 checksums and a leading bracketed release group from a
// name (without extension) and splits the rest into words.
func Tokenize(name string) []string {
	cleaned := checksumPattern.ReplaceAllString(name, " ")
	cleaned = leadingGroupPattern.ReplaceAllString(cleaned, " ")

	return tokenPattern.FindAllString(cleaned, -1)
}

// IsReleaseTag reports whether a token is a release tag that ends a title: a resolution, a
// codec, source, audio or HDR tag, optionally in brackets or followed by "-GROUP".
func IsReleaseTag(token string) bool {
	lower := strings.ToLower(strings.Trim(token, tagBrackets))
	if isBareReleaseTag(lower) {
		return true
	}

	index := strings.LastIndex(lower, releaseGroupSeparator)

	return index > 0 && isBareReleaseTag(lower[:index])
}

func isBareReleaseTag(lower string) bool {
	if releaseTagPattern.MatchString(lower) {
		return true
	}

	match := resolutionPattern.FindStringSubmatch(lower)
	if match == nil {
		return false
	}

	resolution, err := strconv.Atoi(match[1])

	return err == nil && resolution >= minimumResolution && resolution <= maximumResolution
}

// YearOf returns the year a token stands for: "(YYYY)", "[YYYY]" or a bare "YYYY" within
// 1880–2100. bare reports a token without brackets.
func YearOf(token string) (year int, bare, ok bool) {
	match := yearPattern.FindStringSubmatch(token)
	if match == nil {
		return 0, false, false
	}

	value, err := strconv.Atoi(match[1] + match[2] + match[3])
	if err != nil || value < minimumYear || value > maximumYear {
		return 0, false, false
	}

	return value, match[3] != "", true
}

// ParseTitle cleans a name (without extension): the first release tag ends the title, the
// last year-like token is the year, and the words before it are the title. If no title is
// left ("2019.1080p"), the year token is the title and there is no year.
func ParseTitle(name string) TitleYear {
	allTokens := Tokenize(name)
	tokens := allTokens[:firstReleaseTag(allTokens)]

	yearIndex, year := lastYear(tokens)
	if yearIndex < 0 {
		return TitleYear{Title: joinTitle(tokens, name)}
	}

	title := joinTitle(tokens[:yearIndex], "")
	if title == "" {
		return TitleYear{Title: joinTitle(tokens[:yearIndex+1], name)}
	}

	return TitleYear{Title: title, Year: library.NumberOf(year)}
}

func firstReleaseTag(tokens []string) int {
	for index, token := range tokens {
		if IsReleaseTag(token) {
			return index
		}
	}

	return len(tokens)
}

// lastYear finds the last year-like token. A bracketed year counts anywhere; a bare year only
// as the last title word, directly before the release tags or the end of the name ("The
// Matrix 1999", "The.Matrix.1999.1080p"), as Plex, Jellyfin and Kodi do. The rule is purely
// positional, so "Blade Runner 2049" without another year deterministically becomes "Blade
// Runner" (2049), while "Blade Runner 2049 (2017)" keeps 2049 in the title. A lone year stays
// the title (see ParseTitle).
func lastYear(tokens []string) (index, year int) {
	for position := len(tokens) - 1; position >= 0; position-- {
		value, bare, ok := YearOf(tokens[position])
		if ok && (!bare || position == len(tokens)-1) {
			return position, value
		}
	}

	return -1, 0
}

// joinTitle joins the title words, drops dangling punctuation such as a trailing " - " and
// falls back to the trimmed fallback when nothing is left.
func joinTitle(tokens []string, fallback string) string {
	title := strings.TrimFunc(strings.Join(tokens, " "), isTitleEdge)
	if title != "" {
		return title
	}

	return strings.TrimSpace(fallback)
}

func isTitleEdge(character rune) bool {
	return unicode.IsSpace(character) || character == '-'
}
