package enrich

import (
	"context"
	"fmt"
	"time"
)

// ProviderStatus is the state of one provider.
type ProviderStatus struct {
	Provider ProviderName
	State    ProviderState
}

// Status is what /metadata/status reports: the queue length, the last successful
// enrichment (zero when none) and the state of every provider.
type Status struct {
	QueueLength int
	LastSuccess time.Time
	Providers   []ProviderStatus
}

// Status reports the queue. A provider switched off in the settings is disabled whatever its
// health; otherwise its health decides.
func (queue *Queue) Status(ctx context.Context) (Status, error) {
	length, err := queue.deps.Jobs.QueueLength(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("metadata status: %w", err)
	}

	switchedOn, err := queue.switchedOn(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("metadata status: %w", err)
	}

	known := knownProviders()

	providers := make([]ProviderStatus, 0, len(known))
	for _, provider := range known {
		state := ProviderStateDisabled
		if switchedOn(provider) {
			state = queue.deps.Health.State(provider)
		}

		providers = append(providers, ProviderStatus{Provider: provider, State: state})
	}

	return Status{QueueLength: length, LastSuccess: queue.deps.Health.LastSuccess(), Providers: providers}, nil
}

// switchedOn returns which providers the settings allow; without settings all are on.
func (queue *Queue) switchedOn(ctx context.Context) (func(ProviderName) bool, error) {
	if queue.deps.Settings == nil {
		return func(ProviderName) bool { return true }, nil
	}

	settings, err := queue.deps.Settings.OnlineSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("read settings: %w", err)
	}

	return func(provider ProviderName) bool {
		switch provider {
		case ProviderTMDB:
			return settings.Enabled && settings.TMDB
		case ProviderMusicBrainz, ProviderCoverArt:
			return settings.Enabled && settings.MusicBrainz // MusicBrainz covers the Cover Art Archive
		default:
			return false
		}
	}, nil
}
