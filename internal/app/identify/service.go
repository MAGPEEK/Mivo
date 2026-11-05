package identify

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

// Deps are the ports of the Service. A nil lookup counts as a provider that is switched off;
// a nil Logger discards.
type Deps struct {
	Items     ItemReader
	Matches   MatchWriter
	Artwork   ProviderArtworkRemover
	Files     DataFileRemover
	Children  ShowChildren
	Libraries LibraryItems
	Queue     Enqueuer
	Settings  enrich.SettingsSource
	Movies    TitleLookup
	Shows     TitleLookup
	Music     MusicLookup
	Logger    *slog.Logger
}

// Service runs the identify, unmatch and refresh use cases.
type Service struct {
	deps Deps
}

// NewService returns a Service on deps.
func NewService(deps Deps) *Service {
	if deps.Logger == nil {
		deps.Logger = slog.New(slog.DiscardHandler)
	}

	return &Service{deps: deps}
}

// family is the provider side of an item kind: which provider matches it and with which
// reference kind. Seasons and episodes belong to the show family because they are matched
// through their show.
type family struct {
	kind      match.RefKind
	providers []match.RefProvider
	music     bool
}

// familyOf returns the provider family of an item kind and reports false for kinds without
// a provider in milestone 6.
func familyOf(kind library.ItemKind) (family, bool) {
	tmdbProviders := []match.RefProvider{match.RefTMDB, match.RefIMDb}
	musicProviders := []match.RefProvider{match.RefMusicBrainz}

	switch kind {
	case library.ItemKindMovie:
		return family{kind: match.RefKindMovie, providers: tmdbProviders}, true
	case library.ItemKindShow, library.ItemKindSeason, library.ItemKindEpisode:
		return family{kind: match.RefKindShow, providers: tmdbProviders}, true
	case library.ItemKindAlbum:
		return family{kind: match.RefKindRelease, providers: musicProviders, music: true}, true
	case library.ItemKindArtist:
		return family{kind: match.RefKindArtist, providers: musicProviders, music: true}, true
	case library.ItemKindExtra, library.ItemKindTrack, library.ItemKindPhotoAlbum, library.ItemKindPhoto,
		library.ItemKindAudiobook:
		return family{}, false
	default:
		return family{}, false
	}
}

// target is an item a use case works on, with the settings read for it.
type target struct {
	id       string
	snapshot enrich.ItemSnapshot
	family   family
	language string
}

// loadTarget loads an item of a kind with a metadata provider. It fails with
// domain.ErrInvalid for a kind without provider and domain.ErrNotFound for an unknown item.
func (service *Service) loadTarget(ctx context.Context, itemID string) (target, error) {
	snapshot, err := service.deps.Items.ItemSnapshot(ctx, itemID)
	if err != nil {
		return target{}, fmt.Errorf("load item: %w", err)
	}

	itemFamily, ok := familyOf(snapshot.Kind)
	if !ok {
		return target{}, fmt.Errorf("%s items have no metadata provider: %w", snapshot.Kind, domain.ErrInvalid)
	}

	return target{id: itemID, snapshot: snapshot, family: itemFamily}, nil
}

// requireOnline fails with domain.ErrConflict when online metadata is switched off globally,
// for the item's library or for the item's provider, and sets the metadata language.
func (service *Service) requireOnline(ctx context.Context, current *target) error {
	settings, err := service.deps.Settings.OnlineSettings(ctx)
	if err != nil {
		return fmt.Errorf("read online settings: %w", err)
	}

	if !settings.Enabled || !current.snapshot.OnlineEnabled || !service.providerOn(current.family, settings) {
		return fmt.Errorf("online metadata is switched off: %w", domain.ErrConflict)
	}

	current.language = cmp.Or(current.snapshot.Language, settings.Language)

	return nil
}

// loadOnlineTarget loads an item that is about to be matched online, combining loadTarget
// and requireOnline.
func (service *Service) loadOnlineTarget(ctx context.Context, itemID string) (target, error) {
	current, err := service.loadTarget(ctx, itemID)
	if err != nil {
		return target{}, err
	}

	if err := service.requireOnline(ctx, &current); err != nil {
		return target{}, err
	}

	return current, nil
}

// providerOn reports whether the family's provider is switched on and wired.
func (service *Service) providerOn(itemFamily family, settings enrich.OnlineSettings) bool {
	if itemFamily.music {
		return settings.MusicBrainz && service.deps.Music != nil
	}

	if itemFamily.kind == match.RefKindMovie {
		return settings.TMDB && service.deps.Movies != nil
	}

	return settings.TMDB && service.deps.Shows != nil
}

// enqueueAll queues the items, ignoring items deleted since they were listed.
func (service *Service) enqueueAll(ctx context.Context, itemIDs []string, reason enrich.Reason) error {
	for _, itemID := range itemIDs {
		err := service.deps.Queue.Enqueue(ctx, itemID, reason)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("queue item %s: %w", itemID, err)
		}
	}

	return nil
}

// enqueueWithChildren queues the item and, for a show, its seasons and episodes. The item
// itself must exist, so its ErrNotFound is returned.
func (service *Service) enqueueWithChildren(ctx context.Context, current target, reason enrich.Reason) error {
	if err := service.deps.Queue.Enqueue(ctx, current.id, reason); err != nil {
		return fmt.Errorf("queue item: %w", err)
	}

	if current.snapshot.Kind != library.ItemKindShow {
		return nil
	}

	children, err := service.deps.Children.SeasonsAndEpisodes(ctx, current.id)
	if err != nil {
		return fmt.Errorf("list seasons and episodes: %w", err)
	}

	return service.enqueueAll(ctx, children, reason)
}
