package naming

import (
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// Outcome says what the scan does with a file.
type Outcome string

// Known outcomes.
const (
	// OutcomePlaced means the file belongs to the item chain of Result.Placement.
	OutcomePlaced Outcome = "placed"
	// OutcomeIgnored means the file is not catalogued (a sample).
	OutcomeIgnored Outcome = "ignored"
)

// FileContext is what the parser knows about one video file: its library-relative path
// (Unicode NFC, "/"-separated) and how many videos its directory holds, not counting extras
// folders and samples.
type FileContext struct {
	Path                string
	DirectoryVideoCount int
}

// Result is the parser's decision for one file. Placement is empty when the file is ignored.
type Result struct {
	Outcome   Outcome
	Placement library.Placement
}

// Place decides where a video file belongs in a library of the given kind. Shows libraries
// return show, season and episode placements (or show and extra); an unknown kind or an
// invalid path wraps domain.ErrInvalid.
func Place(kind library.Kind, file FileContext) (Result, error) {
	switch kind {
	case library.KindMovies:
		return placeInMovies(file)
	case library.KindShows:
		return placeInShows(file)
	case library.KindMusic:
		return Result{}, fmt.Errorf("place %q: music files go through PlaceMusic: %w", file.Path, domain.ErrInvalid)
	case library.KindPhotos:
		return placeInPhotos(file)
	case library.KindAudiobooks:
		return Result{}, fmt.Errorf("place %q: audiobooks are grouped by GroupAudiobooks: %w", file.Path, domain.ErrInvalid)
	default:
		return Result{}, fmt.Errorf("place %q: unknown library kind %q: %w", file.Path, kind, domain.ErrInvalid)
	}
}

func placeInMovies(file FileContext) (Result, error) {
	if _, err := library.ParseRelativePath(file.Path); err != nil {
		return Result{}, fmt.Errorf("place: %w", err)
	}

	if isSample(file) {
		return Result{Outcome: OutcomeIgnored}, nil
	}

	placement, isExtra := placeExtra(file)
	if !isExtra {
		placement = placeMovie(newMovieFile(file))
	}

	if err := placement.Validate(); err != nil {
		return Result{}, fmt.Errorf("place %q: %w", file.Path, err)
	}

	return Result{Outcome: OutcomePlaced, Placement: placement}, nil
}
