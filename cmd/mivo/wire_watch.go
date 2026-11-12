package main

import (
	"context"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	appaccess "github.com/magpeek/mivo/internal/app/access"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	approws "github.com/magpeek/mivo/internal/app/rows"
	appwatch "github.com/magpeek/mivo/internal/app/watch"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
	"github.com/magpeek/mivo/internal/domain/watch"
)

// episodePageLimit is the page size of the child reads that collect the episodes of a show.
const episodePageLimit = library.MaxPageLimit

// maxEpisodeDepth is how many container levels lie between a show and its episodes: show,
// season, episode. A deeper tree does not exist (ADR 0027 §2); the bound only keeps a faulty
// catalog from looping.
const maxEpisodeDepth = 2

// newWatchService builds the watch state use cases on the sqlite watch store and the scoped
// library service, which decides what the viewer may see.
func newWatchService(db *sqlite.DB, libraries *applibrary.Service) (*appwatch.Service, error) {
	catalog := watchCatalog{libraries: libraries}

	service, err := appwatch.NewService(appwatch.Dependencies{
		Items: catalog, Episodes: catalog, Store: sqlite.NewWatchStore(db), Clock: time.Now,
	})
	if err != nil {
		return nil, fmt.Errorf("create watch service: %w", err)
	}

	return service, nil
}

// watchCatalog adapts the scoped library service to the item and episode ports of
// app/watch. Both reads run as the viewer of the context, so an item the viewer may not see
// is unknown to them.
type watchCatalog struct {
	libraries *applibrary.Service
}

// LookupWatchTarget reads the item as the viewer and describes what the watch state needs.
func (catalog watchCatalog) LookupWatchTarget(ctx context.Context, itemID string) (appwatch.Target, error) {
	detail, err := catalog.libraries.GetItem(ctx, itemID)
	if err != nil {
		return appwatch.Target{}, fmt.Errorf("look up watch target %s: %w", itemID, err)
	}

	return appwatch.Target{
		ItemID: itemID, Kind: detail.Item.Kind, DurationSeconds: detailDuration(detail).Seconds(),
		AttachKey: attachKey(detail),
	}, nil
}

// detailDuration is the catalog length of an item: the end of an audiobook's last chapter
// (the parts add up there), else its primary file's length. Zero means unknown.
func detailDuration(detail applibrary.ItemDetail) time.Duration {
	if len(detail.Chapters) > 0 {
		return detail.Chapters[len(detail.Chapters)-1].Chapter.End
	}

	if len(detail.Files) > 0 {
		return detail.Files[0].Description.Duration
	}

	return 0
}

// attachKey is the item's provider reference for a later re-attach, "<provider>:<kind>:<id>"
// of the first provider the item is matched with; empty when it is unmatched.
func attachKey(detail applibrary.ItemDetail) string {
	for _, provider := range []metadata.Provider{metadata.ProviderTMDB, metadata.ProviderTVDB, metadata.ProviderIMDb} {
		if value, ok := detail.Metadata.ProviderID(provider); ok {
			return fmt.Sprintf("%s:%s:%s", provider, detail.Item.Kind, value)
		}
	}

	return ""
}

// ListEpisodes returns the episodes below a show or season that the viewer of ctx may see. The
// restriction argument is that same viewer's; the child reads take it from the context, so it
// is not applied twice.
func (catalog watchCatalog) ListEpisodes(ctx context.Context, containerID string, _ access.Restriction) ([]appwatch.Target, error) {
	return catalog.collectEpisodes(ctx, containerID, maxEpisodeDepth)
}

// collectEpisodes lists the children of containerID and descends into seasons.
func (catalog watchCatalog) collectEpisodes(ctx context.Context, containerID string, depth int) ([]appwatch.Target, error) {
	var episodes []appwatch.Target

	request := applibrary.ChildPageRequest{Page: library.PageRequest{Limit: episodePageLimit}}

	for {
		page, err := catalog.libraries.ListChildren(ctx, containerID, request)
		if err != nil {
			return nil, fmt.Errorf("list children of %s: %w", containerID, err)
		}

		found, err := catalog.episodesOfPage(ctx, page.Items, depth)
		if err != nil {
			return nil, err
		}

		episodes = append(episodes, found...)

		if page.NextCursor == "" {
			return episodes, nil
		}

		request.Page.Cursor = page.NextCursor
	}
}

func (catalog watchCatalog) episodesOfPage(ctx context.Context, items []applibrary.ItemSummary, depth int) ([]appwatch.Target, error) {
	var episodes []appwatch.Target

	for _, summary := range items {
		if summary.Item.Kind == library.ItemKindEpisode {
			episodes = append(episodes, appwatch.Target{
				ItemID: summary.Item.ID, Kind: summary.Item.Kind,
				DurationSeconds: summary.PrimaryFile.Description.Duration.Seconds(),
			})

			continue
		}

		if summary.Item.Kind != library.ItemKindSeason || depth <= 1 {
			continue
		}

		inner, err := catalog.collectEpisodes(ctx, summary.Item.ID, depth-1)
		if err != nil {
			return nil, err
		}

		episodes = append(episodes, inner...)
	}

	return episodes, nil
}

// watchRowSource adapts the watch store to the rows of app/rows: the store needs the start of
// the Continue watching window and the viewer's restriction, which the rows do not carry.
type watchRowSource struct {
	store *sqlite.WatchStore
	clock func() time.Time
}

var _ approws.WatchRows = watchRowSource{}

// ContinueWatching lists the profile's in-progress items, most recently played first.
func (source watchRowSource) ContinueWatching(ctx context.Context, request approws.WatchRowRequest) ([]string, error) {
	query, err := source.query(ctx, request)
	if err != nil {
		return nil, err
	}

	ids, err := source.store.ContinueWatching(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("continue watching of profile %s: %w", request.ProfileID, err)
	}

	return ids, nil
}

// UpNext lists the next unwatched episode of each show the profile is watching.
func (source watchRowSource) UpNext(ctx context.Context, request approws.WatchRowRequest) ([]string, error) {
	query, err := source.query(ctx, request)
	if err != nil {
		return nil, err
	}

	ids, err := source.store.UpNext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("up next of profile %s: %w", request.ProfileID, err)
	}

	return ids, nil
}

func (source watchRowSource) query(ctx context.Context, request approws.WatchRowRequest) (sqlite.WatchRowQuery, error) {
	restriction, err := appaccess.RestrictionFrom(ctx)
	if err != nil {
		return sqlite.WatchRowQuery{}, fmt.Errorf("watch row restriction: %w", err)
	}

	return sqlite.WatchRowQuery{
		ProfileID: request.ProfileID, LibraryID: request.LibraryID, Limit: request.Limit,
		PlayedSince: source.clock().Add(-watch.ContinueWatchingWindow), Restriction: restriction,
	}, nil
}
