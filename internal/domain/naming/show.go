package naming

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/magpeek/mivo/internal/domain/library"
)

// rootShowKeyPrefix marks the key of a show derived from root-level file names.
const rootShowKeyPrefix = "name:"

// unnumberedKeyPrefix marks the key of an episode without a number (its normalised name).
const unnumberedKeyPrefix = "title:"

// showFolderDepth is the number of leading directories that name the show itself.
const showFolderDepth = 1

// airDatePattern matches "2024-01-05" tokens; air dates are not parsed (ADR 0027 §4).
var airDatePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// showFile is a video file of a shows library split into the parts the rules look at.
type showFile struct {
	path        string
	directories []string
	stem        string
	tokens      []string
}

func newShowFile(file FileContext) showFile {
	segments := strings.Split(file.Path, "/")
	base := segments[len(segments)-1]
	stem := strings.TrimSuffix(base, path.Ext(base))

	return showFile{
		path:        file.Path,
		directories: segments[:len(segments)-1],
		stem:        stem,
		tokens:      Tokenize(stem),
	}
}

func placeInShows(file FileContext) (Result, error) {
	if _, err := library.ParseRelativePath(file.Path); err != nil {
		return Result{}, fmt.Errorf("place: %w", err)
	}

	if isSample(file) {
		return Result{Outcome: OutcomeIgnored}, nil
	}

	parsed := newShowFile(file)

	placement, isExtra := placeShowExtra(parsed)
	if !isExtra {
		placement = placeEpisode(parsed)
	}

	if err := placement.Validate(); err != nil {
		return Result{}, fmt.Errorf("place %q: %w", file.Path, err)
	}

	return Result{Outcome: OutcomePlaced, Placement: placement}, nil
}

// placeShowExtra attaches files below an extras folder (inside the show or a season) to the
// show.
func placeShowExtra(file showFile) (library.Placement, bool) {
	if len(file.directories) <= showFolderDepth {
		return library.Placement{}, false
	}

	for _, directory := range file.directories[showFolderDepth:] {
		if extraType, ok := extraFolders[strings.ToLower(directory)]; ok {
			show := showLevelFromDirectory(file.directories[0])

			return extraPlacement(show, file.path, file.stem, extraType), true
		}
	}

	return library.Placement{}, false
}

// placeEpisode places an episode file: a show folder file takes its show from the folder, a
// root file from the text before its marker.
func placeEpisode(file showFile) library.Placement {
	inFolder := len(file.directories) > 0
	firstBare := 1

	if inFolder {
		firstBare = 0
	}

	marker, found := findMarker(file.tokens, firstBare)

	var show library.Level

	var folders []string

	if inFolder {
		show = showLevelFromDirectory(file.directories[0])
		folders = file.directories[showFolderDepth:]
	} else {
		show = rootShowLevel(file, marker, found)
	}

	season := seasonFor(marker, found, folders)
	episode, label := episodeFor(file, marker, found)

	return library.Placement{
		Levels:       []library.Level{show, seasonLevel(season), episode},
		VersionLabel: label,
	}
}

func seasonFor(marker episodeMarker, found bool, folders []string) int {
	if found {
		if number, ok := marker.season.Get(); ok {
			return number
		}
	}

	if number, ok := seasonOfFolders(folders); ok {
		return number
	}

	return defaultSeason
}

// episodeFor builds the episode level and the version label of a file. A file without a
// marker is an unnumbered episode keyed by its normalised name.
func episodeFor(file showFile, marker episodeMarker, found bool) (library.Level, string) {
	if found {
		title, label := splitEpisodeName(wordsAfterMarker(file.tokens, marker), marker.number)

		return episodeLevel(marker, title), label
	}

	title := ParseTitle(file.stem).Title
	if title == "" {
		title = file.stem
	}

	if title == "" {
		title = strings.TrimSpace(episodeTitlePrefix)
	}

	return library.Level{
		Kind:     library.ItemKindEpisode,
		MatchKey: unnumberedKeyPrefix + library.NormalizeKey(file.stem),
		Title:    title,
	}, ""
}

// showLevelFromDirectory is the show a top-level folder stands for.
func showLevelFromDirectory(directory string) library.Level {
	parsed := ParseTitle(directory)

	return library.Level{
		Kind:     library.ItemKindShow,
		MatchKey: library.NormalizeKey(directory),
		Title:    parsed.Title,
		Year:     parsed.Year,
	}
}

// rootShowLevel derives the show of a root-level file from the text before its episode
// marker (or its air date, or the whole name).
func rootShowLevel(file showFile, marker episodeMarker, found bool) library.Level {
	end := len(file.tokens)

	if found {
		end = marker.index
	} else {
		for index, token := range file.tokens {
			if airDatePattern.MatchString(token) {
				end = index

				break
			}
		}
	}

	parsed := ParseTitle(strings.Join(file.tokens[:end], " "))
	if parsed.Title == "" {
		parsed.Title = file.stem
	}

	return library.Level{
		Kind:     library.ItemKindShow,
		MatchKey: rootShowKeyPrefix + library.NormalizeKey(parsed.Title),
		Title:    parsed.Title,
		Year:     parsed.Year,
	}
}
