// Package posterproxy serves the poster previews of identify candidates (M6-13) under Mivo
// URLs, so the browser never contacts a provider host (ADR 0010, ADR 0031). The search
// handler registers each candidate poster and gets an opaque, short-lived ID; the route
// GET /api/v1/metadata/posters/{posterId} fetches the picture once from the TMDB image host
// through the hardened outbound client and keeps it in memory until the ID expires. Nothing
// of the browser's request (headers, cookies) reaches the provider: the source is asked with
// the provider reference only.
package posterproxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
)

// Defaults of the proxy (ADR 0031: previews are short-lived and bounded).
const (
	// DefaultLifetime is how long an ID stays valid after its last registration.
	DefaultLifetime = time.Hour
	// DefaultMaxEntries bounds the registered IDs; the one expiring first is dropped.
	DefaultMaxEntries = 512
	// DefaultMaxCachedBytes bounds the picture bytes kept in memory; a picture beyond the
	// budget is served but not kept.
	DefaultMaxCachedBytes = 32 << 20

	// idBytes is the entropy of an ID; it is sent as 32 lower-case hex digits.
	idBytes = 16
	// pathPrefix is where the route serves; PosterURL appends the ID.
	pathPrefix = "/api/v1/metadata/posters/"
)

// Source downloads a TMDB picture without storing it; *artworkfetch.Fetcher implements it
// with the TMDB image host allow-list. Errors wrap domain.ErrNotFound, ErrInvalid,
// ErrForbidden, ErrUnavailable or ErrRateLimited.
type Source interface {
	// Preview returns the picture bytes.
	Preview(ctx context.Context, ref enrich.ArtworkRef) ([]byte, error)
}

// Options configure a Proxy. Source, Settings and Logger are required; zero values of the
// rest mean the defaults, a nil Clock time.Now. Settings (*metadatasettings.Service) is read
// before every download: with online metadata or TMDB switched off the proxy serves only
// what it already holds and opens no connection (ADR 0031 §2).
type Options struct {
	Source         Source
	Settings       enrich.SettingsSource
	Logger         *slog.Logger
	Clock          func() time.Time
	Lifetime       time.Duration
	MaxEntries     int
	MaxCachedBytes int
}

// entry is one registered poster. fetch is a one-slot lock that serialises the download, so
// concurrent requests for one ID reach the provider once; a waiter gives up when its request
// ends, so a hanging download never holds the other requests (lockFetch).
type entry struct {
	ref     enrich.ArtworkRef
	expires time.Time
	fetch   chan struct{}
	body    []byte
}

func newEntry(ref enrich.ArtworkRef, expires time.Time) *entry {
	return &entry{ref: ref, expires: expires, fetch: make(chan struct{}, 1)}
}

// lockFetch takes the fetch lock or fails with the context's error.
func (found *entry) lockFetch(ctx context.Context) error {
	select {
	case found.fetch <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for poster download: %w", ctx.Err())
	}
}

func (found *entry) unlockFetch() {
	<-found.fetch
}

// Proxy registers candidate posters and serves them. It is safe for concurrent use and
// implements restapi.CandidatePosters.
type Proxy struct {
	options Options

	mutex       sync.Mutex
	entries     map[string]*entry
	idsByRef    map[enrich.ArtworkRef]string
	cachedBytes int
}

// Compile-time check: the search handler registers posters here.
var _ restapi.CandidatePosters = (*Proxy)(nil)

// New returns a Proxy. A missing Source, Settings or Logger is a wiring bug and wraps
// domain.ErrInvalid.
func New(options Options) (*Proxy, error) {
	switch {
	case options.Source == nil:
		return nil, fmt.Errorf("new poster proxy: missing source: %w", domain.ErrInvalid)
	case options.Settings == nil:
		return nil, fmt.Errorf("new poster proxy: missing settings: %w", domain.ErrInvalid)
	case options.Logger == nil:
		return nil, fmt.Errorf("new poster proxy: missing logger: %w", domain.ErrInvalid)
	}

	if options.Clock == nil {
		options.Clock = time.Now
	}

	options.Lifetime = orDefault(options.Lifetime, DefaultLifetime)
	options.MaxEntries = orDefault(options.MaxEntries, DefaultMaxEntries)
	options.MaxCachedBytes = orDefault(options.MaxCachedBytes, DefaultMaxCachedBytes)

	return &Proxy{options: options, entries: map[string]*entry{}, idsByRef: map[enrich.ArtworkRef]string{}}, nil
}

func orDefault[T time.Duration | int](value, fallback T) T {
	if value <= 0 {
		return fallback
	}

	return value
}

// PosterURL registers a TMDB poster and returns its server URL. The same reference keeps
// its ID (and cached bytes) while registered; registering it again extends its lifetime.
// Pictures of other providers are not proxied and get "".
func (proxy *Proxy) PosterURL(ref enrich.ArtworkRef) string {
	if ref.Provider != enrich.ArtworkProviderTMDB || ref.Path == "" {
		return ""
	}

	proxy.mutex.Lock()
	defer proxy.mutex.Unlock()

	now := proxy.options.Clock()
	proxy.dropExpired(now)

	if id, ok := proxy.idsByRef[ref]; ok {
		proxy.entries[id].expires = now.Add(proxy.options.Lifetime)

		return pathPrefix + id
	}

	id, err := newID()
	if err != nil {
		proxy.options.Logger.Warn("poster preview id unavailable", slog.Any("error", err))

		return ""
	}

	if len(proxy.entries) >= proxy.options.MaxEntries {
		proxy.dropSoonestExpiring()
	}

	proxy.entries[id] = newEntry(ref, now.Add(proxy.options.Lifetime))
	proxy.idsByRef[ref] = id

	return pathPrefix + id
}

func newID() (string, error) {
	random := make([]byte, idBytes)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("read random id: %w", err)
	}

	return hex.EncodeToString(random), nil
}

// lookup returns the live entry of an ID.
func (proxy *Proxy) lookup(id string) (*entry, bool) {
	proxy.mutex.Lock()
	defer proxy.mutex.Unlock()

	proxy.dropExpired(proxy.options.Clock())
	found, ok := proxy.entries[id]

	return found, ok
}

// keep stores fetched bytes while they fit the memory budget; the caller holds the entry's
// fetch lock.
func (proxy *Proxy) keep(id string, found *entry, body []byte) {
	proxy.mutex.Lock()
	defer proxy.mutex.Unlock()

	if proxy.entries[id] != found || proxy.cachedBytes+len(body) > proxy.options.MaxCachedBytes {
		return
	}

	found.body = body
	proxy.cachedBytes += len(body)
}

// dropExpired removes expired entries; the caller holds the mutex.
func (proxy *Proxy) dropExpired(now time.Time) {
	for id, found := range proxy.entries {
		if !now.Before(found.expires) {
			proxy.remove(id, found)
		}
	}
}

// dropSoonestExpiring removes the entry that expires first; the caller holds the mutex.
func (proxy *Proxy) dropSoonestExpiring() {
	var (
		soonestID string
		soonest   *entry
	)

	for id, found := range proxy.entries {
		if soonest == nil || found.expires.Before(soonest.expires) {
			soonestID, soonest = id, found
		}
	}

	if soonest != nil {
		proxy.remove(soonestID, soonest)
	}
}

func (proxy *Proxy) remove(id string, found *entry) {
	delete(proxy.entries, id)
	delete(proxy.idsByRef, found.ref)
	proxy.cachedBytes -= len(found.body)
}
