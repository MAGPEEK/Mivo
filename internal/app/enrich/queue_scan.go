package enrich

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// retryFailedTimeout bounds making failed jobs due after a settings change; the listener
// interface carries no context.
const retryFailedTimeout = 5 * time.Second

// BatchEnqueuer queues the items a scan refreshed. Implemented by adapters/sqlite.
type BatchEnqueuer interface {
	// EnqueueItems queues those of itemIDs that are of one of kinds, belong to a library
	// with online metadata and whose match is not disabled, with the dedupe of Enqueue, and
	// returns how many it queued.
	EnqueueItems(ctx context.Context, itemIDs []string, kinds []library.ItemKind, reason Reason) (int, error)
}

// FailedJobRetrier makes jobs that failed before due now, so a job that backed off while a
// provider was refused or switched off does not wait hours after the settings changed.
// Implemented by adapters/sqlite.
type FailedJobRetrier interface {
	// RetryFailedJobs makes every failed job that is not running due now and returns how
	// many it changed.
	RetryFailedJobs(ctx context.Context) (int, error)
}

// EnqueueScanned queues the items a scan refreshed (reason scan) unless online metadata is
// switched off; only the kinds of a provider that is switched on are queued. It implements
// the scan's EnrichmentRequester and never waits for the network.
func (queue *Queue) EnqueueScanned(ctx context.Context, itemIDs []string) error {
	if queue.deps.Batch == nil {
		return fmt.Errorf("enqueue scanned items: no batch store: %w", domain.ErrInvalid)
	}

	kinds, err := queue.enrichableKinds(ctx)
	if err != nil {
		return fmt.Errorf("enqueue scanned items: %w", err)
	}

	if len(kinds) == 0 || len(itemIDs) == 0 {
		return nil
	}

	queued, err := queue.deps.Batch.EnqueueItems(ctx, itemIDs, kinds, ReasonScan)
	if err != nil {
		return fmt.Errorf("enqueue scanned items: %w", err)
	}

	if queued > 0 {
		queue.signal()
	}

	return nil
}

// enrichableKinds returns the item kinds a provider that is switched on can match; none when
// online metadata is off. Without Settings every provider counts as on.
func (queue *Queue) enrichableKinds(ctx context.Context) ([]library.ItemKind, error) {
	settings := OnlineSettings{Enabled: true, TMDB: true, MusicBrainz: true}

	if queue.deps.Settings != nil {
		var err error

		settings, err = queue.deps.Settings.OnlineSettings(ctx)
		if err != nil {
			return nil, fmt.Errorf("read settings: %w", err)
		}
	}

	return ProviderKinds(settings), nil
}

// ProviderKinds returns the item kinds the switched-on providers match (ADR 0031 §1): TMDB
// movies, shows, seasons and episodes, MusicBrainz albums and artists. Nothing when online
// metadata is off.
func ProviderKinds(settings OnlineSettings) []library.ItemKind {
	if !settings.Enabled {
		return nil
	}

	var kinds []library.ItemKind

	if settings.TMDB {
		kinds = append(kinds, library.ItemKindMovie, library.ItemKindShow, library.ItemKindSeason, library.ItemKindEpisode)
	}

	if settings.MusicBrainz {
		kinds = append(kinds, library.ItemKindAlbum, library.ItemKindArtist)
	}

	return kinds
}

// SettingsChanged forgets the provider states (a refused key is tried again), makes the
// jobs that backed off due now and wakes the worker. It implements the settings service's
// ChangeListener.
func (queue *Queue) SettingsChanged() {
	queue.deps.Health.SettingsChanged()

	if queue.deps.Retries != nil {
		ctx, cancel := context.WithTimeout(context.Background(), retryFailedTimeout)
		defer cancel()

		if _, err := queue.deps.Retries.RetryFailedJobs(ctx); err != nil {
			queue.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "could not retry failed metadata jobs",
				slog.String("error", err.Error()))
		}
	}

	queue.signal()
}
