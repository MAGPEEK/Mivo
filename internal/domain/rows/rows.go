// Package rows holds the pure rules of the Home and Overview rows (tasks M5-12, M5-13): which
// rows a library has, their card shapes and identifiers, the grouping of recently added
// episodes into show or season cards, the daily genre rotation and the hint of an empty or
// sparse response.
package rows

import (
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/watch"
)

// Kind is what a row shows (API RowKind).
type Kind string

// The row kinds of milestone 5; later milestones add more (API RowKind).
const (
	KindRecentlyAdded  Kind = "recentlyAdded"
	KindNewReleases    Kind = "newReleases"
	KindMoreGenre      Kind = "moreGenre"
	KindRecentEpisodes Kind = "recentEpisodes"
	// KindContinueWatching and KindUpNext are the profile's watch rows (M7-19); they have no
	// "See all".
	KindContinueWatching Kind = "continueWatching"
	KindUpNext           Kind = "upNext"
)

// CardFormat is the card shape of a row (API CardFormat).
type CardFormat string

// The card formats.
const (
	CardPoster  CardFormat = "poster"
	CardEpisode CardFormat = "episode"
	CardSquare  CardFormat = "square"
)

// Hint says why a rows response has no or few rows (API RowsHint); HintNone sends none.
type Hint string

// The hints.
const (
	HintNone          Hint = ""
	HintNoLibraries   Hint = "noLibraries"
	HintNoItems       Hint = "noItems"
	HintScanning      Hint = "scanning"
	HintNotEnoughData Hint = "notEnoughData"
)

// Translation keys of the row titles (API Row.titleKey) and their parameter names.
const (
	TitleRecentlyAdded    = "rows.recentlyAdded"
	TitleRecentlyAddedIn  = "rows.recentlyAddedIn"
	TitleNewReleases      = "rows.newReleases"
	TitleMoreGenre        = "rows.moreGenre"
	TitleRecentEpisodes   = "rows.recentEpisodes"
	TitleContinueWatching = "rows.continueWatching"
	TitleUpNext           = "rows.upNext"
	ParamLibrary          = "library"
	ParamGenre            = "genre"
)

// DefaultItemLimit is the number of items per row when the client asks for none (API
// RowItemLimit default).
const DefaultItemLimit = 20

// HomeLeadKinds returns the watch rows that open Home, in display order, before the
// "recentlyAdded" rows of the libraries.
func HomeLeadKinds() []Kind {
	return []Kind{KindContinueWatching, KindUpNext}
}

// OverviewLeadKinds returns the watch rows that open a library's Overview, before the rows of
// OverviewKinds: only "continueWatching"; "upNext" belongs to Home.
func OverviewLeadKinds() []Kind {
	return []Kind{KindContinueWatching}
}

// WatchRowLimit returns how many items a watch row asks for: the requested row limit, capped
// at the row kind's maximum; zero for a kind that is no watch row.
func WatchRowLimit(kind Kind, requested int) int {
	switch kind {
	case KindContinueWatching:
		return min(requested, watch.ContinueWatchingLimit)
	case KindUpNext:
		return min(requested, watch.UpNextLimit)
	case KindRecentlyAdded, KindNewReleases, KindMoreGenre, KindRecentEpisodes:
		return 0
	default:
		return 0
	}
}

// rowIDSeparator joins the parts of a row ID; the API pattern allows it.
const rowIDSeparator = ":"

// RowID returns the stable ID of a row of one library: "<kind>:<library ID>".
func RowID(kind Kind, libraryID string) string {
	return string(kind) + rowIDSeparator + libraryID
}

// WatchRowID returns the stable ID of a watch row: the bare kind on Home (libraryID empty),
// else "<kind>:<library ID>".
func WatchRowID(kind Kind, libraryID string) string {
	if libraryID == "" {
		return string(kind)
	}

	return RowID(kind, libraryID)
}

// GenreRowID returns the stable ID of a "moreGenre" row. A genre name may hold characters
// the API's ID pattern forbids, so the ID carries a hash of its case-folded name: the same
// genre keeps its ID across requests and days, whatever its spelling in the catalog.
func GenreRowID(libraryID, genre string) string {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(strings.ToLower(genre))) // a hash never fails to write

	return RowID(KindMoreGenre, libraryID) + rowIDSeparator + strconv.FormatUint(hash.Sum64(), 16)
}

// OverviewKinds returns the row kinds of a library's Overview in display order. A
// "moreGenre" entry stands for up to MaxGenreRows rows. Movies and shows have release dates
// and genres; audiobooks have genres; music and photos show only what was added.
func OverviewKinds(kind library.Kind) []Kind {
	switch kind {
	case library.KindMovies:
		return []Kind{KindRecentlyAdded, KindNewReleases, KindMoreGenre}
	case library.KindShows:
		return []Kind{KindRecentlyAdded, KindNewReleases, KindMoreGenre, KindRecentEpisodes}
	case library.KindAudiobooks:
		return []Kind{KindRecentlyAdded, KindMoreGenre}
	case library.KindMusic, library.KindPhotos:
		return []Kind{KindRecentlyAdded}
	default:
		return []Kind{KindRecentlyAdded}
	}
}

// ListedItemKind returns the item kind the rows of a library list: albums in a music library
// (a new album matters more than its artist), else the library's default top-level kind;
// empty for a library without one.
func ListedItemKind(kind library.Kind) library.ItemKind {
	if kind == library.KindMusic {
		return library.ItemKindAlbum
	}

	kinds := kind.TopLevelItemKinds()
	if len(kinds) == 0 {
		return ""
	}

	return kinds[0]
}

// GroupsEpisodes reports whether "recentlyAdded" of the library groups new episodes into show
// or season cards (HO-05) instead of listing top-level items.
func GroupsEpisodes(kind library.Kind) bool {
	return kind == library.KindShows
}

// CardFormatOf returns the card shape of a row: episode cards for "recentEpisodes", square
// cards for albums and photos, posters for everything else.
func CardFormatOf(rowKind Kind, libraryKind library.Kind) CardFormat {
	switch {
	case rowKind == KindContinueWatching || rowKind == KindUpNext:
		return CardPoster // the API gives both watch rows poster cards
	case rowKind == KindRecentEpisodes:
		return CardEpisode
	case libraryKind == library.KindMusic || libraryKind == library.KindPhotos:
		return CardSquare
	default:
		return CardPoster
	}
}

// LibraryState is what a library's hint depends on.
type LibraryState struct {
	// HasItems is true when the library has an available item.
	HasItems bool
	// Scanning is true while a scan of the library runs.
	Scanning bool
	// MissingMetadata is true when a row the library should have is missing for lack of
	// release dates or genres.
	MissingMetadata bool
}

// LibraryHint returns the hint of a library's Overview: "scanning" or "noItems" without
// items, "notEnoughData" when rows are missing for lack of metadata, else none.
func LibraryHint(state LibraryState) Hint {
	switch {
	case !state.HasItems && state.Scanning:
		return HintScanning
	case !state.HasItems:
		return HintNoItems
	case state.MissingMetadata:
		return HintNotEnoughData
	default:
		return HintNone
	}
}

// HomeHint returns the hint of Home: "noLibraries" without libraries; when no library has
// items, "scanning" while one is scanned, else "noItems"; otherwise none.
func HomeHint(states []LibraryState) Hint {
	if len(states) == 0 {
		return HintNoLibraries
	}

	scanning := false

	for _, state := range states {
		if state.HasItems {
			return HintNone
		}

		scanning = scanning || state.Scanning
	}

	if scanning {
		return HintScanning
	}

	return HintNoItems
}
