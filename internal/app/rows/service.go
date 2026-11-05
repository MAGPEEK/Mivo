package rows

import (
	"errors"
	"fmt"
	"time"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/rows"
)

// MaxItemLimit is the most items a row carries (API RowItemLimit maximum).
const MaxItemLimit = 50

// Item is one card of a row: the item's summary plus what a row adds to it. SeasonNumber
// is set on episode cards, ShowTitle on season and episode cards, NewEpisodeCount on the
// grouped show and season cards of "recentlyAdded".
type Item struct {
	Summary         applibrary.ItemSummary
	SeasonNumber    library.OptionalNumber
	ShowTitle       string
	NewEpisodeCount int
}

// Row is one row with its first items. SeeAll is the Browse pivot query of the row's library
// that "See all" opens: only its Order and its Filter's Kind and Genres are set.
type Row struct {
	ID          string
	Kind        rows.Kind
	TitleKey    string
	TitleParams map[string]string
	LibraryID   string
	Genre       string
	CardFormat  rows.CardFormat
	SeeAll      library.ListingQuery
	Items       []Item
}

// HeroItem is one title of the Home hero; Logo is nil without a logo. Genres (at most
// MaxHeroGenres) and PrimaryFile feed the meta line; a show has no primary file (zero ID).
type HeroItem struct {
	ItemID      string
	LibraryID   string
	Kind        library.ItemKind
	Title       string
	Year        library.OptionalNumber
	SummaryLine string
	Backdrop    artwork.Ref
	Logo        *artwork.Ref
	Genres      []string
	PrimaryFile media.File
}

// Home is the Home screen: its rows, the hero and the hint (HintNone for none).
type Home struct {
	Rows []Row
	Hero []HeroItem
	Hint rows.Hint
}

// LibraryRows is the Overview pivot of a library.
type LibraryRows struct {
	LibraryID string
	Rows      []Row
	Hint      rows.Hint
}

// Dependencies lists everything Service needs; every field is required.
type Dependencies struct {
	Libraries LibrarySource
	Rows      RowReader
	Summaries SummaryReader
	Catalog   Catalog
	Scans     ScanStatus
	// Clock tells the date that seeds the genre rotation; the server's local time.
	Clock func() time.Time
}

// Service generates the rows. It is safe for concurrent use.
type Service struct {
	libraries LibrarySource
	rows      RowReader
	summaries SummaryReader
	catalog   Catalog
	scans     ScanStatus
	clock     func() time.Time
}

// NewService validates the dependencies and returns a Service. A missing dependency is a
// wiring bug and is reported as an error wrapping domain.ErrInvalid.
func NewService(dependencies Dependencies) (*Service, error) {
	if err := dependencies.validate(); err != nil {
		return nil, fmt.Errorf("new rows service: %w", err)
	}

	return &Service{
		libraries: dependencies.Libraries, rows: dependencies.Rows, summaries: dependencies.Summaries,
		catalog: dependencies.Catalog, scans: dependencies.Scans, clock: dependencies.Clock,
	}, nil
}

func (dependencies Dependencies) validate() error {
	required := []struct {
		name    string
		present bool
	}{
		{name: "library source", present: dependencies.Libraries != nil},
		{name: "row reader", present: dependencies.Rows != nil},
		{name: "summary reader", present: dependencies.Summaries != nil},
		{name: "catalog", present: dependencies.Catalog != nil},
		{name: "scan status", present: dependencies.Scans != nil},
		{name: "clock", present: dependencies.Clock != nil},
	}

	var problems []error

	for _, dependency := range required {
		if !dependency.present {
			problems = append(problems, fmt.Errorf("missing dependency %s: %w", dependency.name, domain.ErrInvalid))
		}
	}

	return errors.Join(problems...)
}

// itemLimit returns the items per row: 0 means rows.DefaultItemLimit, anything outside 1 to
// MaxItemLimit wraps domain.ErrInvalid.
func itemLimit(limit int) (int, error) {
	switch {
	case limit == 0:
		return rows.DefaultItemLimit, nil
	case limit < 1 || limit > MaxItemLimit:
		return 0, fmt.Errorf("row item limit %d not between 1 and %d: %w", limit, MaxItemLimit, domain.ErrInvalid)
	default:
		return limit, nil
	}
}
