package naming

import (
	"path"
	"strings"

	"github.com/magpeek/mivo/internal/domain/library"
)

// Match key parts of movies (ADR 0027 §3/§4).
const (
	folderMovieKeyPrefix = "dir:"
	looseKeySeparator    = "|"
	rootDirectory        = "."
	onlyVideo            = 1
)

// movieFile is a video file split into the parts the movie rules look at.
type movieFile struct {
	directory  string
	stem       string
	base       string
	videoCount int
}

func newMovieFile(file FileContext) movieFile {
	base := path.Base(file.Path)

	return movieFile{
		directory:  path.Dir(file.Path),
		stem:       strings.TrimSuffix(base, path.Ext(base)),
		base:       base,
		videoCount: file.DirectoryVideoCount,
	}
}

// placeMovie places a (non-extra, non-sample) video file as a folder movie or a loose movie.
func placeMovie(file movieFile) library.Placement {
	markers := extractVersionMarkers(file.stem)

	if file.directory != rootDirectory {
		folderKey := library.NormalizeKey(path.Base(file.directory))
		if remainder, ok := cutNamePrefix(markers.rest, folderKey); ok {
			return folderMoviePlacement(file.directory, versionLabel(remainder, markers.edition), markers.part)
		}

		if file.videoCount == onlyVideo {
			return folderMoviePlacement(file.directory, markers.edition, markers.part)
		}
	}

	return looseMoviePlacement(file, markers)
}

func folderMoviePlacement(directory, label string, part library.OptionalNumber) library.Placement {
	return library.Placement{
		Levels:       []library.Level{folderMovieLevel(directory)},
		VersionLabel: label,
		PartNumber:   part,
	}
}

// folderMovieLevel is the movie a directory stands for; title and year come from its name.
func folderMovieLevel(directory string) library.Level {
	parsed := ParseTitle(path.Base(directory))

	return library.Level{
		Kind:     library.ItemKindMovie,
		MatchKey: folderMovieKeyPrefix + library.NormalizeKey(directory),
		Title:    parsed.Title,
		Year:     parsed.Year,
	}
}

func looseMoviePlacement(file movieFile, markers versionMarkers) library.Placement {
	directory := file.directory
	parsed := ParseTitle(markers.rest)
	if parsed.Title == "" {
		parsed.Title = file.base
	}

	directoryKey := ""
	if directory != rootDirectory {
		directoryKey = library.NormalizeKey(directory)
	}

	key := directoryKey + looseKeySeparator + library.NormalizeKey(parsed.Title) +
		looseKeySeparator + parsed.Year.String()

	return library.Placement{
		Levels: []library.Level{{
			Kind: library.ItemKindMovie, MatchKey: key, Title: parsed.Title, Year: parsed.Year,
		}},
		VersionLabel: markers.edition,
		PartNumber:   markers.part,
	}
}
