package artwork

import (
	"path"
	"strconv"
	"strings"
)

// LocalContextKind says which item a folder's image files belong to.
type LocalContextKind string

// Local contexts of ADR 0027 §8.
const (
	// ContextFolderMovie is a movie that has a folder of its own.
	ContextFolderMovie LocalContextKind = "folder_movie"
	// ContextLooseMovie is a movie file next to other movies; only names with its base
	// name belong to it, a folder-wide poster.jpg does not.
	ContextLooseMovie LocalContextKind = "loose_movie"
	// ContextShowFolder is the top folder of a show.
	ContextShowFolder LocalContextKind = "show_folder"
	// ContextSeasonFolder is a season folder inside a show.
	ContextSeasonFolder LocalContextKind = "season_folder"
	// ContextEpisode is one episode file; only names with its base name belong to it.
	ContextEpisode LocalContextKind = "episode"
)

// LocalContext describes the item whose folder is classified. BaseName is the video file
// name without extension (folder movie, loose movie, episode); Season is the number of a
// season folder.
type LocalContext struct {
	Kind     LocalContextKind
	BaseName string
	Season   int
}

// LocalMatch is the classification of one image file. Priority orders files of the same
// kind (lower wins). ForSeason marks a season poster; Season is then its season number (0
// for specials), which in a show folder comes from the file name.
type LocalMatch struct {
	Kind      Kind
	Priority  int
	ForSeason bool
	Season    int
}

// Name patterns of local artwork.
const (
	basePlaceholder       = "<base>"
	seasonPosterPrefix    = "season"
	seasonPosterSuffix    = "-poster"
	specialsPosterStem    = "season-specials-poster"
	specialsSeasonNumber  = 0
	seasonNumberMaxDigits = 4
)

// localRule maps a lowercase file name stem to an artwork kind. A stem containing
// basePlaceholder applies only with a known base name.
type localRule struct {
	stem string
	kind Kind
}

// ClassifyLocal reports whether an image file in the folder of the given context is artwork
// of that item, and of which kind and priority (ADR 0027 §8). Names and extensions (jpg,
// jpeg, png, webp, tbn) are compared case-insensitively, because SMB shares and exFAT do not
// preserve case reliably. Fixed names are checked before names built from the base name. In
// a show folder, seasonNN-poster and season-specials-poster are season posters; they rank
// after a poster inside the season folder itself.
func ClassifyLocal(context LocalContext, fileName string) (LocalMatch, bool) {
	stem, isImage := imageStem(fileName)
	if !isImage {
		return LocalMatch{}, false
	}

	if context.Kind == ContextShowFolder {
		if season, isSeasonPoster := seasonPosterNumber(stem); isSeasonPoster {
			return LocalMatch{Kind: KindPoster, Priority: len(seasonFolderRules()), ForSeason: true, Season: season}, true
		}
	}

	match, found := matchRules(rulesFor(context.Kind), stem, strings.ToLower(context.BaseName))
	if found && context.Kind == ContextSeasonFolder {
		match.ForSeason = true
		match.Season = context.Season
	}

	return match, found
}

// imageStem returns the lowercase file name without extension if the name is a plain file
// name with an image extension.
func imageStem(fileName string) (string, bool) {
	if strings.ContainsAny(fileName, `/\`) {
		return "", false
	}

	lower := strings.ToLower(fileName)
	extension := path.Ext(lower)
	stem := strings.TrimSuffix(lower, extension)

	switch extension {
	case ".jpg", ".jpeg", ".png", ".webp", ".tbn":
		return stem, stem != ""
	default:
		return "", false
	}
}

func seasonPosterNumber(stem string) (int, bool) {
	if stem == specialsPosterStem {
		return specialsSeasonNumber, true
	}

	digits, hasPrefix := strings.CutPrefix(stem, seasonPosterPrefix)
	digits, hasSuffix := strings.CutSuffix(digits, seasonPosterSuffix)

	if !hasPrefix || !hasSuffix || len(digits) > seasonNumberMaxDigits || !isASCIIDigits(digits) {
		return 0, false
	}

	season, err := strconv.Atoi(digits)

	return season, err == nil
}

func matchRules(rules []localRule, stem, baseName string) (LocalMatch, bool) {
	isFixedMatch := func(rule localRule) bool {
		return !strings.Contains(rule.stem, basePlaceholder) && rule.stem == stem
	}
	isBaseMatch := func(rule localRule) bool {
		return baseName != "" && strings.Contains(rule.stem, basePlaceholder) &&
			strings.Replace(rule.stem, basePlaceholder, baseName, 1) == stem
	}

	if match, found := findRule(rules, isFixedMatch); found {
		return match, true
	}

	return findRule(rules, isBaseMatch)
}

// findRule returns the first matching rule; its priority is its position among the rules of
// the same kind, so the rule lists below read in priority order.
func findRule(rules []localRule, matches func(localRule) bool) (LocalMatch, bool) {
	for index, rule := range rules {
		if !matches(rule) {
			continue
		}

		priority := 0

		for _, earlier := range rules[:index] {
			if earlier.kind == rule.kind {
				priority++
			}
		}

		return LocalMatch{Kind: rule.kind, Priority: priority}, true
	}

	return LocalMatch{}, false
}

func rulesFor(contextKind LocalContextKind) []localRule {
	switch contextKind {
	case ContextFolderMovie:
		return movieRules()
	case ContextLooseMovie:
		return baseNameRules(movieRules())
	case ContextShowFolder:
		return showFolderRules()
	case ContextSeasonFolder:
		return seasonFolderRules()
	case ContextEpisode:
		return episodeRules()
	default:
		return nil
	}
}

func movieRules() []localRule {
	return []localRule{
		{"poster", KindPoster},
		{"folder", KindPoster},
		{"cover", KindPoster},
		{"movie", KindPoster},
		{"<base>-poster", KindPoster},
		{"<base>", KindPoster},
		{"fanart", KindBackdrop},
		{"backdrop", KindBackdrop},
		{"background", KindBackdrop},
		{"art", KindBackdrop},
		{"<base>-fanart", KindBackdrop},
		{"landscape", KindThumb},
		{"thumb", KindThumb},
		{"<base>-thumb", KindThumb},
		{"clearlogo", KindLogo},
		{"logo", KindLogo},
		{"banner", KindBanner},
	}
}

func showFolderRules() []localRule {
	return []localRule{
		{"poster", KindPoster},
		{"folder", KindPoster},
		{"fanart", KindBackdrop},
		{"backdrop", KindBackdrop},
		{"clearlogo", KindLogo},
		{"logo", KindLogo},
		{"banner", KindBanner},
	}
}

func seasonFolderRules() []localRule {
	return []localRule{{"poster", KindPoster}, {"folder", KindPoster}}
}

func episodeRules() []localRule {
	return []localRule{{"<base>-thumb", KindThumb}, {"<base>", KindThumb}}
}

// baseNameRules keeps only the rules built from the base name, in their order.
func baseNameRules(rules []localRule) []localRule {
	kept := make([]localRule, 0, len(rules))

	for _, rule := range rules {
		if strings.Contains(rule.stem, basePlaceholder) {
			kept = append(kept, rule)
		}
	}

	return kept
}

func isASCIIDigits(text string) bool {
	if text == "" {
		return false
	}

	for index := range len(text) {
		if text[index] < '0' || text[index] > '9' {
			return false
		}
	}

	return true
}
