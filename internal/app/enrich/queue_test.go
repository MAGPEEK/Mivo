package enrich_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
)

var errQueueStore = errors.New("queue store broken")

func TestReasonIsValid(t *testing.T) {
	t.Parallel()

	for _, reason := range []enrich.Reason{enrich.ReasonScan, enrich.ReasonRefresh, enrich.ReasonIdentify, enrich.ReasonPeriodic} {
		if !reason.IsValid() {
			t.Errorf("%q invalid, want valid", reason)
		}
	}

	if enrich.Reason("nightly").IsValid() {
		t.Error(`"nightly" valid, want invalid`)
	}
}

func TestQueueEnqueueDedupesAndNeverBlocks(t *testing.T) {
	t.Parallel()

	jobs := newFakeJobs(newFakeClock())
	queue := enrich.NewQueue(enrich.QueueDeps{Jobs: jobs, Health: enrich.NewHealth(enrich.HealthOptions{})})

	// Nobody reads the wake-ups: repeated requests must still return at once.
	for range 3 {
		if err := queue.Enqueue(context.Background(), "item-a", enrich.ReasonScan); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}

	if err := queue.Enqueue(context.Background(), "item-b", enrich.ReasonRefresh); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if got := jobs.length(); got != 2 {
		t.Fatalf("queued jobs = %d, want 2", got)
	}
}

func TestQueueEnqueueErrors(t *testing.T) {
	t.Parallel()

	jobs := newFakeJobs(newFakeClock())
	queue := enrich.NewQueue(enrich.QueueDeps{Jobs: jobs, Health: enrich.NewHealth(enrich.HealthOptions{})})

	err := queue.Enqueue(context.Background(), "item-a", enrich.Reason("nightly"))
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("unknown reason: err = %v, want ErrInvalid", err)
	}

	if jobs.length() != 0 {
		t.Fatal("an invalid request reached the store")
	}

	jobs.enqueueErr = errQueueStore

	if err := queue.Enqueue(context.Background(), "item-a", enrich.ReasonScan); !errors.Is(err, errQueueStore) {
		t.Fatalf("store failure: err = %v, want it wrapped", err)
	}
}

func TestQueueStatus(t *testing.T) {
	t.Parallel()

	clock := newFakeClock()
	jobs := newFakeJobs(clock)
	health := enrich.NewHealth(enrich.HealthOptions{})
	settings := fakeSettings{settings: enrich.OnlineSettings{Enabled: true, TMDB: true, MusicBrainz: false}}
	queue := enrich.NewQueue(enrich.QueueDeps{Jobs: jobs, Health: health, Settings: settings})

	mustEnqueue(t, queue, "item-a")
	health.Observe(enrich.ProviderTMDB, errUnavailable())

	status, err := queue.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	want := []enrich.ProviderStatus{
		{Provider: enrich.ProviderTMDB, State: enrich.ProviderStateOffline},
		{Provider: enrich.ProviderMusicBrainz, State: enrich.ProviderStateDisabled},
		{Provider: enrich.ProviderCoverArt, State: enrich.ProviderStateDisabled},
	}
	if status.QueueLength != 1 || !status.LastSuccess.IsZero() || !equalStatuses(status.Providers, want) {
		t.Fatalf("status = %+v, want length 1, no success, providers %+v", status, want)
	}
}

func TestQueueStatusWithoutSettingsUsesHealth(t *testing.T) {
	t.Parallel()

	queue := enrich.NewQueue(enrich.QueueDeps{Jobs: newFakeJobs(newFakeClock()), Health: enrich.NewHealth(enrich.HealthOptions{})})

	status, err := queue.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	for _, provider := range status.Providers {
		if provider.State != enrich.ProviderStateOK {
			t.Errorf("%s = %s, want ok", provider.Provider, provider.State)
		}
	}
}

func TestQueueStatusErrors(t *testing.T) {
	t.Parallel()

	jobs := newFakeJobs(newFakeClock())
	jobs.lengthErr = errQueueStore
	queue := enrich.NewQueue(enrich.QueueDeps{Jobs: jobs, Health: enrich.NewHealth(enrich.HealthOptions{})})

	if _, err := queue.Status(context.Background()); !errors.Is(err, errQueueStore) {
		t.Fatalf("length failure: err = %v, want it wrapped", err)
	}

	errSettings := errors.New("settings broken")
	queue = enrich.NewQueue(enrich.QueueDeps{
		Jobs: newFakeJobs(newFakeClock()), Health: enrich.NewHealth(enrich.HealthOptions{}),
		Settings: fakeSettings{err: errSettings},
	})

	if _, err := queue.Status(context.Background()); !errors.Is(err, errSettings) {
		t.Fatalf("settings failure: err = %v, want it wrapped", err)
	}
}

func TestBackoffDoublesUpToCap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		attempts int
		want     time.Duration
	}{
		{attempts: -1, want: time.Minute},
		{attempts: 0, want: time.Minute},
		{attempts: 1, want: 2 * time.Minute},
		{attempts: 4, want: 16 * time.Minute},
		{attempts: 8, want: 256 * time.Minute},
		{attempts: 9, want: enrich.MaxBackoff},
		{attempts: 1000, want: enrich.MaxBackoff},
	}

	for _, test := range tests {
		if got := enrich.Backoff(test.attempts); got != test.want {
			t.Errorf("Backoff(%d) = %v, want %v", test.attempts, got, test.want)
		}
	}
}

func mustEnqueue(t *testing.T, queue *enrich.Queue, itemIDs ...string) {
	t.Helper()

	for _, itemID := range itemIDs {
		if err := queue.Enqueue(context.Background(), itemID, enrich.ReasonScan); err != nil {
			t.Fatalf("Enqueue %s: %v", itemID, err)
		}
	}
}

func equalStatuses(got, want []enrich.ProviderStatus) bool {
	if len(got) != len(want) {
		return false
	}

	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}

	return true
}

func errUnavailable() error {
	return errors.Join(errors.New("dial tcp: no route"), domain.ErrUnavailable)
}
