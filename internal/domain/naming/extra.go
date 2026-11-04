package naming

import (
	"path"
	"regexp"
	"strings"

	"github.com/magpeek/mivo/internal/domain/library"
)

// trailerName is the whole file name that makes a file in a movie folder a trailer.
const trailerName = "trailer"

// sampleFolderName marks a folder whose videos are all samples (compared case-folded).
const sampleFolderName = "sample"

// sampleNamePattern matches a sample stem: "sample" as the whole name or a "-sample" /
// ".sample" suffix (ADR 0027 §4).
var sampleNamePattern = regexp.MustCompile(`(?i)(?:^|[-.])sample$`)

// extraFolders maps case-folded extras folder names to their extra type.
//
//nolint:gochecknoglobals // read-only lookup table, built once like the compiled patterns
var extraFolders = map[string]library.ExtraType{
	"trailers":          library.ExtraTypeTrailer,
	"featurettes":       library.ExtraTypeFeaturette,
	"behind the scenes": library.ExtraTypeBehindTheScenes,
	"deleted scenes":    library.ExtraTypeDeletedScene,
	"interviews":        library.ExtraTypeInterview,
	"scenes":            library.ExtraTypeScene,
	"shorts":            library.ExtraTypeShort,
	"other":             library.ExtraTypeOther,
	"extras":            library.ExtraTypeOther,
}

// extraSuffix is a file name suffix that marks an extra of the movie the rest names.
type extraSuffix struct {
	suffix    string
	extraType library.ExtraType
}

// extraSuffixes lists the suffixes of ADR 0027 §4.
//
//nolint:gochecknoglobals // read-only lookup table, built once like the compiled patterns
var extraSuffixes = []extraSuffix{
	{"-trailer", library.ExtraTypeTrailer},
	{"-featurette", library.ExtraTypeFeaturette},
	{"-behindthescenes", library.ExtraTypeBehindTheScenes},
	{"-deleted", library.ExtraTypeDeletedScene},
	{"-interview", library.ExtraTypeInterview},
	{"-scene", library.ExtraTypeScene},
	{"-short", library.ExtraTypeShort},
	{"-other", library.ExtraTypeOther},
}

// isSample reports a sample file: named "sample", ending in "-sample"/".sample" or inside a
// "Sample" folder.
func isSample(file FileContext) bool {
	segments := strings.Split(file.Path, "/")
	for _, directory := range segments[:len(segments)-1] {
		if strings.EqualFold(directory, sampleFolderName) {
			return true
		}
	}

	return sampleNamePattern.MatchString(newMovieFile(file).stem)
}

// placeExtra places a file as an extra if a rule of ADR 0027 §4 says it is one.
func placeExtra(file FileContext) (library.Placement, bool) {
	if placement, ok := placeFolderExtra(file); ok {
		return placement, true
	}

	movie := newMovieFile(file)

	if strings.EqualFold(movie.stem, trailerName) && movie.directory != rootDirectory {
		return extraPlacement(folderMovieLevel(movie.directory), file.Path, movie.stem, library.ExtraTypeTrailer), true
	}

	candidate, base, ok := cutExtraSuffix(movie)
	if !ok {
		return library.Placement{}, false
	}

	movie.stem = base

	return extraPlacement(suffixExtraParent(movie), file.Path, base, candidate.extraType), true
}

// cutExtraSuffix finds an extra suffix and returns the base name before it. A suffix is no
// extra marker when it names nothing in the library root ("-trailer.mkv") or is a release
// group after a release tag ("Movie.2010.1080p.x264-SHORT").
func cutExtraSuffix(movie movieFile) (extraSuffix, string, bool) {
	lowerStem := strings.ToLower(movie.stem)

	for _, candidate := range extraSuffixes {
		if !strings.HasSuffix(lowerStem, candidate.suffix) {
			continue
		}

		base := movie.stem[:len(movie.stem)-len(candidate.suffix)]
		tokens := Tokenize(base)

		switch {
		case len(tokens) == 0 && movie.directory == rootDirectory:
			return extraSuffix{}, "", false
		case len(tokens) > 0 && IsReleaseTag(tokens[len(tokens)-1]):
			return extraSuffix{}, "", false
		default:
			return candidate, base, true
		}
	}

	return extraSuffix{}, "", false
}

// suffixExtraParent is the movie the base name places to; a bare suffix such as
// "-trailer.mkv" names no movie, so in a movie folder it belongs to the folder movie.
func suffixExtraParent(movie movieFile) library.Level {
	if len(Tokenize(movie.stem)) == 0 {
		return folderMovieLevel(movie.directory)
	}

	return placeMovie(movie).Levels[0]
}

// placeFolderExtra handles a file below an extras folder whose parent is a movie folder: a
// directory below the root whose name carries a year ("Movie (2010)/Trailers"). The parser
// cannot see the file system, so the year is the evidence; extras folders in the root or in
// collection folders ("Blender Open Movies/Extras") stay normal folders.
func placeFolderExtra(file FileContext) (library.Placement, bool) {
	segments := strings.Split(file.Path, "/")

	for index := 1; index < len(segments)-1; index++ {
		extraType, ok := extraFolders[strings.ToLower(segments[index])]
		if !ok {
			continue
		}

		parent := folderMovieLevel(path.Join(segments[:index]...))
		if !parent.Year.IsSet() {
			return library.Placement{}, false
		}

		return extraPlacement(parent, file.Path, newMovieFile(file).stem, extraType), true
	}

	return library.Placement{}, false
}

// extraPlacement builds movie level + extra level. The extra's key is its normalised
// library-relative path: unique per file and stable while the file stays where it is.
func extraPlacement(parent library.Level, filePath, name string, extraType library.ExtraType) library.Placement {
	title := ParseTitle(name).Title
	if title == "" {
		title = extraType.String()
	}

	extra := library.Level{
		Kind:      library.ItemKindExtra,
		MatchKey:  library.NormalizeKey(filePath),
		Title:     title,
		ExtraType: extraType,
	}

	return library.Placement{Levels: []library.Level{parent, extra}}
}
