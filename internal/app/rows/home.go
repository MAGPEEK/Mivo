package rows

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	appaccess "github.com/magpeek/mivo/internal/app/access"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/rows"
)

// Bounds of the hero (API HomeRows.hero): at most MaxHeroItems titles, chosen among the
// heroCandidatesPerLibrary newest top-level items of each library; at most maxHeroLookups
// detail reads, so a catalog without backdrops costs a bounded number of reads.
const (
	MaxHeroItems             = 5
	heroCandidatesPerLibrary = 2 * MaxHeroItems
	maxHeroLookups           = 3 * MaxHeroItems
	// MaxHeroGenres bounds the genres of a hero's meta line (API HeroItem.genres).
	MaxHeroGenres = 3
)

// MaxHomeRows is the most rows Home carries (API HomeRows.rows maxItems); libraries after
// that many filled rows get no row.
const MaxHomeRows = 100

// Home returns the rows of the Home screen: the profile's "continueWatching" and "upNext"
// rows (M7-19; none without a profile or when empty), then one "recentlyAdded" row per visible library in
// library order (the decision of task M5-12: "recentEpisodes" stays on the library
// Overview), at most MaxHomeRows, the hero and the hint. Libraries without items have no
// row. A row's title parameter is the library name, which library.MaxNameLength keeps within
// the API's 200 characters. A limit as in LibraryRows.
func (service *Service) Home(ctx context.Context, limit int) (Home, error) {
	restriction, limit, err := homeRequest(ctx, limit)
	if err != nil {
		return Home{}, fmt.Errorf("home rows: %w", err)
	}

	libraries, err := service.libraries.VisibleLibraries(ctx)
	if err != nil {
		return Home{}, fmt.Errorf("home rows: %w", err)
	}

	lead, err := service.watchRows(ctx, rows.HomeLeadKinds(), watchScope{restriction: restriction, limit: limit})
	if err != nil {
		return Home{}, fmt.Errorf("home rows: %w", err)
	}

	home := Home{Rows: append([]Row{}, lead...), Hero: []HeroItem{}}

	states := make([]rows.LibraryState, 0, len(libraries))

	var filled []target

	for _, managed := range libraries {
		if len(home.Rows) >= MaxHomeRows {
			break
		}

		source := service.targetOf(ctx, managed, limit, restriction)

		row, state, err := service.homeLibraryRow(ctx, source)
		if err != nil {
			return Home{}, fmt.Errorf("home rows: %w", err)
		}

		states = append(states, state)

		if state.HasItems {
			home.Rows = append(home.Rows, row)
			filled = append(filled, source)
		}
	}

	home.Hero, err = service.hero(ctx, filled, resumable(lead))
	if err != nil {
		return Home{}, fmt.Errorf("home rows: %w", err)
	}

	home.Hint = rows.HomeHint(states)

	return home, nil
}

// homeRequest returns the viewer's restriction and the validated item limit.
func homeRequest(ctx context.Context, limit int) (access.Restriction, int, error) {
	restriction, err := appaccess.RestrictionFrom(ctx)
	if err != nil {
		return access.Restriction{}, 0, err
	}

	limit, err = itemLimit(limit)
	if err != nil {
		return access.Restriction{}, 0, err
	}

	return restriction, limit, nil
}

// homeLibraryRow builds a library's "recentlyAdded" row of Home and the library's state.
func (service *Service) homeLibraryRow(ctx context.Context, source target) (Row, rows.LibraryState, error) {
	params := map[string]string{rows.ParamLibrary: source.library.Name}

	row, err := service.recentlyAdded(ctx, source, rows.TitleRecentlyAddedIn, params)
	if err != nil {
		return Row{}, rows.LibraryState{}, fmt.Errorf("library %q: %w", source.library.ID, err)
	}

	return row, service.stateOf(source, row), nil
}

// resumable returns the movies and shows of the "continueWatching" row among the lead rows;
// the hero prefers them (HO-01). Episodes are not hero titles.
func resumable(lead []Row) []applibrary.ItemSummary {
	var preferred []applibrary.ItemSummary

	for _, row := range lead {
		if row.Kind != rows.KindContinueWatching {
			continue
		}

		for _, item := range row.Items {
			if kind := item.Summary.Item.Kind; kind == library.ItemKindMovie || kind == library.ItemKindShow {
				preferred = append(preferred, item.Summary)
			}
		}
	}

	return preferred
}

// hero picks the titles the viewer is watching that have a backdrop, then the most recently
// added movies and shows that have one.
func (service *Service) hero(ctx context.Context, libraries []target, preferred []applibrary.ItemSummary) ([]HeroItem, error) {
	recent, err := service.heroCandidates(ctx, libraries)
	if err != nil {
		return nil, err
	}

	candidates := slices.Clone(preferred)
	for _, candidate := range recent {
		if !slices.ContainsFunc(preferred, func(item applibrary.ItemSummary) bool { return item.Item.ID == candidate.Item.ID }) {
			candidates = append(candidates, candidate)
		}
	}

	hero := []HeroItem{}

	for _, candidate := range candidates[:min(maxHeroLookups, len(candidates))] {
		detail, err := service.catalog.GetItem(ctx, candidate.Item.ID)
		if errors.Is(err, domain.ErrNotFound) {
			continue // deleted since the candidates were read
		}

		if err != nil {
			return nil, fmt.Errorf("hero: %w", err)
		}

		if item, found := heroItemOf(candidate, detail); found {
			hero = append(hero, item)
			if len(hero) == MaxHeroItems {
				break
			}
		}
	}

	return hero, nil
}

// heroCandidates returns the newest movies and shows of the libraries, newest first (ties by
// ID).
func (service *Service) heroCandidates(ctx context.Context, libraries []target) ([]applibrary.ItemSummary, error) {
	var candidates []applibrary.ItemSummary

	for _, source := range libraries {
		kind := rows.ListedItemKind(source.library.Kind)
		if kind != library.ItemKindMovie && kind != library.ItemKindShow {
			continue
		}

		query := listedQuery(source, library.SortByDateAdded)
		query.Filter.OfflineRootIDs = source.offline
		query.Restriction = source.restriction

		summaries, err := service.rows.ListRowItems(ctx, ItemRequest{LibraryID: source.library.ID, Query: query, Limit: heroCandidatesPerLibrary})
		if err != nil {
			return nil, fmt.Errorf("hero candidates of library %q: %w", source.library.ID, err)
		}

		candidates = append(candidates, summaries...)
	}

	slices.SortStableFunc(candidates, func(left, right applibrary.ItemSummary) int {
		return cmp.Or(right.Item.CreatedAt.Compare(left.Item.CreatedAt), cmp.Compare(left.Item.ID, right.Item.ID))
	})

	return candidates, nil
}

// heroItemOf returns the hero entry of an item; found is false without a backdrop. The
// primary file comes from the listing, so the meta line costs no extra read.
func heroItemOf(candidate applibrary.ItemSummary, detail applibrary.ItemDetail) (HeroItem, bool) {
	backdrop, found := detail.Artwork[artwork.KindBackdrop]
	if !found {
		return HeroItem{}, false
	}

	item := HeroItem{
		ItemID: detail.Item.ID, LibraryID: detail.Item.LibraryID, Kind: detail.Item.Kind, Title: detail.Item.Title,
		Year: detail.Item.Year, SummaryLine: rows.SummaryLine(detail.Metadata.Summary), Backdrop: backdrop,
		Genres:      slices.Clone(detail.Metadata.Genres[:min(MaxHeroGenres, len(detail.Metadata.Genres))]),
		PrimaryFile: candidate.PrimaryFile,
	}

	if logo, found := detail.Artwork[artwork.KindLogo]; found {
		item.Logo = &logo
	}

	return item, true
}
