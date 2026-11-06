package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/posterproxy"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/adapters/metadata/artworkfetch"
	"github.com/magpeek/mivo/internal/adapters/metadata/coverart"
	"github.com/magpeek/mivo/internal/adapters/metadata/musicbrainz"
	"github.com/magpeek/mivo/internal/adapters/metadata/tmdb"
	"github.com/magpeek/mivo/internal/adapters/netaccess/outbound"
	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/app/identify"
	"github.com/magpeek/mivo/internal/app/metadatasettings"
)

// Request rates of ADR 0031 §7 that the provider adapters do not publish themselves.
const (
	tmdbRequestsPerSecond     = 20
	tmdbRequestBurst          = 20
	coverArtRequestsPerSecond = 1
	coverArtRequestBurst      = 1
)

// metadataHooks are test seams of the online metadata wiring (runHooks.metadata), never
// documented configuration: base URLs of fake providers, the extra hosts they listen on,
// plain HTTP to loopback, a resolver that records every lookup and a short worker poll.
type metadataHooks struct {
	tmdbBaseURL           string
	tmdbImageBaseURL      string
	musicBrainzBaseURL    string
	extraHosts            []string
	allowInsecureLoopback bool
	resolver              outbound.Resolver
	pollInterval          time.Duration
}

// onlineMetadata is the wired online metadata feature (ADR 0031): the settings, the queue
// with its worker and periodic refresh, the identify use cases and the poster proxy.
type onlineMetadata struct {
	settings *metadatasettings.Service
	store    *sqlite.EnrichmentStore
	queue    *enrich.Queue
	worker   *enrich.Worker
	periodic *enrich.PeriodicRefresh
	identify *identify.Service
	posters  *posterproxy.Proxy
}

// apiServices are the online metadata services of the REST API.
func (online onlineMetadata) apiServices() restapi.MetadataServices {
	return restapi.MetadataServices{
		Identify: online.identify, Refresh: online.identify, Settings: online.settings,
		Status: online.queue, Matches: online.store, Posters: online.posters,
	}
}

// settingsRelay passes settings changes to the queue, which is built after the settings
// service because it reads the settings itself.
type settingsRelay struct {
	queue *enrich.Queue
}

// SettingsChanged implements metadatasettings.ChangeListener.
func (relay *settingsRelay) SettingsChanged() {
	if relay.queue != nil {
		relay.queue.SettingsChanged()
	}
}

// providerLookups are the caching provider ports the enricher and identify share.
type providerLookups struct {
	movies  *enrich.CachedMovies
	shows   *enrich.CachedShows
	music   *enrich.CachedMusic
	artwork *enrich.ObservedArtwork
	fetcher *artworkfetch.Fetcher
}

// newOnlineMetadata wires the feature. The adapters are always built, but none opens a
// connection on its own: every call first reads the settings, so with online metadata off
// nothing is dialled (the network guard test checks it) and switching it on in Settings
// works without a restart.
func (app application) newOnlineMetadata(ctx context.Context, db *sqlite.DB) (onlineMetadata, error) {
	relay := &settingsRelay{}

	settings, err := app.newMetadataSettings(ctx, db, relay)
	if err != nil {
		return onlineMetadata{}, err
	}

	store := sqlite.NewEnrichmentStore(db, time.Now)
	health := enrich.NewHealth(enrich.HealthOptions{IsInvalidKey: func(err error) bool { return errors.Is(err, tmdb.ErrInvalidKey) }})
	queue := enrich.NewQueue(enrich.QueueDeps{
		Jobs: store, Health: health, Settings: settings, Batch: store, Retries: store, Logger: app.logger,
	})
	relay.queue = queue

	lookups, err := app.newProviderLookups(settings, store, health)
	if err != nil {
		return onlineMetadata{}, err
	}

	posters, err := posterproxy.New(posterproxy.Options{Source: lookups.fetcher, Settings: settings, Logger: app.logger})
	if err != nil {
		return onlineMetadata{}, fmt.Errorf("create poster proxy: %w", err)
	}

	return onlineMetadata{
		settings: settings, store: store, queue: queue, posters: posters,
		worker: app.newEnrichmentWorker(enrichmentCore{
			store: store, settings: settings, queue: queue, health: health,
		}, lookups),
		periodic: enrich.NewScheduler(store, queue).Periodic(enrich.PeriodicOptions{Logger: app.logger}),
		identify: identify.NewService(identify.Deps{
			Items: store, Matches: store, Artwork: store, Files: lookups.fetcher, Children: store,
			Libraries: store, Queue: queue, Settings: settings,
			Movies: lookups.movies, Shows: lookups.shows, Music: lookups.music, Logger: app.logger,
		}),
	}, nil
}

// newMetadataSettings starts from the configuration, or from the state saved in Settings ›
// Metadata when there is one, so a change made in the UI survives a restart. A switch the
// configuration turns off stays off at start (ADR 0018, notes M6-14).
func (app application) newMetadataSettings(ctx context.Context, db *sqlite.DB,
	listener metadatasettings.ChangeListener,
) (*metadatasettings.Service, error) {
	store := sqlite.NewMetadataSettingsStore(db, time.Now)
	configured := app.config.Metadata

	stored, found, err := store.LoadMetadataSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("load metadata settings: %w", err)
	}

	initial := metadatasettings.StartState(metadatasettings.State{
		Online: configured.Online, TMDB: configured.TMDB.Enabled, MusicBrainz: configured.MusicBrainz.Enabled,
		OwnTMDBKey: string(configured.TMDB.APIKey),
	}, stored, found)

	return metadatasettings.NewService(metadatasettings.Options{
		Initial: initial, DefaultLanguage: configured.Language, BundledTMDBKey: app.config.BundledTMDBKey(),
		Store: store, Listener: listener,
	}), nil
}

// newProviderLookups builds one hardened outbound client per provider with its allowed
// hosts, redirect hosts and rate (ADR 0031 §1, §7) and wraps the adapters in the provider
// cache and health. The Cover Art Archive client follows redirects to *.archive.org only.
func (app application) newProviderLookups(settings *metadatasettings.Service, store *sqlite.EnrichmentStore,
	health *enrich.Health,
) (providerLookups, error) {
	hooks := app.hooks.metadata
	tmdbHosts := app.allowedHosts(tmdb.AllowedHosts())
	tmdbHTTP := app.outboundClient(outbound.Options{
		AllowedHosts: tmdbHosts, RedirectHosts: tmdbHosts,
		DefaultRate: outbound.Rate{PerSecond: tmdbRequestsPerSecond, Burst: tmdbRequestBurst},
	})
	musicHTTP := app.outboundClient(outbound.Options{
		AllowedHosts: app.allowedHosts(musicbrainz.AllowedHosts()), HostRates: musicbrainz.HostRates(),
	})
	coverArtHTTP := app.outboundClient(outbound.Options{
		AllowedHosts:  app.allowedHosts(coverart.AllowedHosts()),
		RedirectHosts: app.allowedHosts(slices.Concat(coverart.RedirectHosts(), artworkfetch.CoverArtRedirectHosts())),
		DefaultRate:   outbound.Rate{PerSecond: coverArtRequestsPerSecond, Burst: coverArtRequestBurst},
	})

	tmdbClient := tmdb.New(tmdb.Options{
		HTTP: tmdbHTTP, Keys: settings, BaseURL: hooks.tmdbBaseURL, ImageBaseURL: hooks.tmdbImageBaseURL,
	})

	fetcher, err := artworkfetch.New(artworkfetch.Options{
		DataDir: app.config.Paths.Data,
		TMDB: artworkfetch.Provider{
			Client: tmdbHTTP, ImageURL: tmdbClient.ImageURL, AllowedHosts: app.allowedHosts(artworkfetch.TMDBHosts()),
		},
		CoverArt:              artworkfetch.Provider{Client: coverArtHTTP, AllowedHosts: app.allowedHosts(artworkfetch.CoverArtHosts())},
		AllowInsecureLoopback: hooks.allowInsecureLoopback,
	})
	if err != nil {
		return providerLookups{}, fmt.Errorf("create artwork fetcher: %w", err)
	}

	cache := enrich.CacheDeps{Cache: store, Health: health, Logger: app.logger}
	music := musicbrainz.New(musicbrainz.Options{HTTP: musicHTTP, BaseURL: hooks.musicBrainzBaseURL})

	return providerLookups{
		movies:  enrich.NewCachedMovies(tmdbClient.Movies(), cache),
		shows:   enrich.NewCachedShows(tmdbClient.Shows(), cache),
		music:   enrich.NewCachedMusic(music, cache),
		artwork: enrich.NewObservedArtwork(fetcher, health),
		fetcher: fetcher,
	}, nil
}

// outboundClient completes the options every provider client shares.
func (app application) outboundClient(options outbound.Options) *outbound.Client {
	options.Version = app.build.Version
	options.AllowInsecureLoopback = app.hooks.metadata.allowInsecureLoopback
	options.Resolver = app.hooks.metadata.resolver

	return outbound.New(options)
}

// allowedHosts adds the hosts of the test fakes to a provider's list.
func (app application) allowedHosts(hosts []string) []string {
	return slices.Concat(hosts, app.hooks.metadata.extraHosts)
}

// enrichmentCore are the parts the worker, the identify use cases and the API share.
type enrichmentCore struct {
	store    *sqlite.EnrichmentStore
	settings *metadatasettings.Service
	queue    *enrich.Queue
	health   *enrich.Health
}

func (app application) newEnrichmentWorker(core enrichmentCore, lookups providerLookups) *enrich.Worker {
	enricher := enrich.NewEnricher(enrich.Deps{
		Store: core.store, Settings: core.settings, Movies: lookups.movies, Shows: lookups.shows,
		Music: lookups.music, Artwork: lookups.artwork, Logger: app.logger,
	})

	return enrich.NewWorker(
		enrich.WorkerDeps{Jobs: core.store, Enricher: enricher, Health: core.health, Queue: core.queue, Logger: app.logger},
		enrich.WorkerOptions{PollInterval: app.hooks.metadata.pollInterval},
	)
}
