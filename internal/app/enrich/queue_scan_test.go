package enrich_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// fakeBatch records EnqueueItems calls.
type fakeBatch struct {
	itemIDs []string
	kinds   []library.ItemKind
	reason  enrich.Reason
	calls   int
	queued  int
	err     error
}

func (batch *fakeBatch) EnqueueItems(_ context.Context, itemIDs []string, kinds []library.ItemKind,
	reason enrich.Reason,
) (int, error) {
	batch.calls++
	batch.itemIDs, batch.kinds, batch.reason = itemIDs, kinds, reason

	return batch.queued, batch.err
}

// fakeRetrier counts RetryFailedJobs calls.
type fakeRetrier struct {
	calls int
	err   error
}

func (retrier *fakeRetrier) RetryFailedJobs(context.Context) (int, error) {
	retrier.calls++

	return 0, retrier.err
}

func TestQueueEnqueueScannedQueuesTheKindsOfTheSwitchedOnProviders(t *testing.T) {
	t.Parallel()

	tmdbKinds := []library.ItemKind{library.ItemKindMovie, library.ItemKindShow, library.ItemKindSeason, library.ItemKindEpisode}
	musicKinds := []library.ItemKind{library.ItemKindAlbum, library.ItemKindArtist}

	tests := []struct {
		name      string
		settings  enrich.OnlineSettings
		wantKinds []library.ItemKind
	}{
		{name: "all on", settings: enrich.OnlineSettings{Enabled: true, TMDB: true, MusicBrainz: true}, wantKinds: slices.Concat(tmdbKinds, musicKinds)},
		{name: "tmdb only", settings: enrich.OnlineSettings{Enabled: true, TMDB: true}, wantKinds: tmdbKinds},
		{name: "musicbrainz only", settings: enrich.OnlineSettings{Enabled: true, MusicBrainz: true}, wantKinds: musicKinds},
		{name: "online off", settings: enrich.OnlineSettings{TMDB: true, MusicBrainz: true}},
		{name: "every provider off", settings: enrich.OnlineSettings{Enabled: true}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			batch := &fakeBatch{queued: 1}
			queue := enrich.NewQueue(enrich.QueueDeps{Batch: batch, Settings: fakeSettings{settings: test.settings}})

			if err := queue.EnqueueScanned(context.Background(), []string{"a", "b"}); err != nil {
				t.Fatalf("EnqueueScanned() error = %v", err)
			}

			if test.wantKinds == nil {
				if batch.calls != 0 {
					t.Errorf("EnqueueItems calls = %d, want none", batch.calls)
				}

				return
			}

			if !slices.Equal(batch.kinds, test.wantKinds) || batch.reason != enrich.ReasonScan ||
				!slices.Equal(batch.itemIDs, []string{"a", "b"}) {
				t.Errorf("EnqueueItems(%v, %v, %s), want the items, kinds %v and reason scan",
					batch.itemIDs, batch.kinds, batch.reason, test.wantKinds)
			}
		})
	}
}

func TestQueueEnqueueScannedErrors(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")

	tests := []struct {
		name    string
		deps    enrich.QueueDeps
		wantErr error
	}{
		{name: "no batch store", deps: enrich.QueueDeps{}, wantErr: domain.ErrInvalid},
		{name: "settings fail", deps: enrich.QueueDeps{Batch: &fakeBatch{}, Settings: fakeSettings{err: boom}}, wantErr: boom},
		{name: "store fails", deps: enrich.QueueDeps{Batch: &fakeBatch{err: boom}}, wantErr: boom},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := enrich.NewQueue(test.deps).EnqueueScanned(context.Background(), []string{"a"})
			if !errors.Is(err, test.wantErr) {
				t.Errorf("EnqueueScanned() error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestQueueEnqueueScannedWithoutItemsAsksNothing(t *testing.T) {
	t.Parallel()

	batch := &fakeBatch{}
	if err := enrich.NewQueue(enrich.QueueDeps{Batch: batch}).EnqueueScanned(context.Background(), nil); err != nil {
		t.Fatalf("EnqueueScanned() error = %v", err)
	}

	if batch.calls != 0 {
		t.Errorf("EnqueueItems calls = %d, want none", batch.calls)
	}
}

func TestQueueSettingsChangedForgetsStatesAndRetriesFailedJobs(t *testing.T) {
	t.Parallel()

	for _, retryErr := range []error{nil, errors.New("database locked")} {
		health := enrich.NewHealth(enrich.HealthOptions{IsInvalidKey: func(error) bool { return true }})
		health.Observe(enrich.ProviderTMDB, errors.New("refused"))

		retrier := &fakeRetrier{err: retryErr}
		queue := enrich.NewQueue(enrich.QueueDeps{Health: health, Retries: retrier})

		queue.SettingsChanged()

		if !health.Usable(enrich.ProviderTMDB) || retrier.calls != 1 {
			t.Errorf("after SettingsChanged (retry error %v): usable %v, retries %d; want usable and one retry",
				retryErr, health.Usable(enrich.ProviderTMDB), retrier.calls)
		}
	}
}

func TestQueueSettingsChangedWithoutRetrier(t *testing.T) {
	t.Parallel()

	enrich.NewQueue(enrich.QueueDeps{}).SettingsChanged()
}
