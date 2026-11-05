package enrich

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

// keySeparator joins the parts of a cache key. Keys hold no path, file name or item ID:
// only what the provider is told (ADR 0031 §5).
const keySeparator = "|"

// cacheKey quotes every part before joining, so a title containing the separator or a quote
// cannot make two different queries share a key ("a|b", "c" versus "a", "b|c").
func cacheKey(parts ...string) string {
	quoted := make([]string, len(parts))
	for index, part := range parts {
		quoted[index] = strconv.Quote(part)
	}

	return strings.Join(quoted, keySeparator)
}

func refKey(ref match.ProviderRef) string {
	return cacheKey(string(ref.Provider), string(ref.Kind), ref.ID)
}

// CachedMovies is a MovieLookup that reads through the provider cache.
type CachedMovies struct {
	inner   MovieLookup
	through readThrough
}

// NewCachedMovies wraps the TMDB movie lookup.
func NewCachedMovies(inner MovieLookup, deps CacheDeps) *CachedMovies {
	return &CachedMovies{inner: inner, through: newReadThrough(deps, ProviderTMDB)}
}

// Search returns the cached or fetched candidates.
func (movies *CachedMovies) Search(ctx context.Context, query SearchQuery) ([]match.Candidate, error) {
	key := cacheKey("movie-search", query.Language, strconv.Itoa(query.Year), query.Title)

	return cachedCall(ctx, slotOf(movies.through, key, candidatesCodec()), func(ctx context.Context) ([]match.Candidate, error) {
		return movies.inner.Search(ctx, query)
	})
}

// Details returns the cached or fetched movie.
func (movies *CachedMovies) Details(ctx context.Context, request DetailsRequest) (Details, error) {
	key := cacheKey("movie-details", refKey(request.Ref), request.Language)

	return cachedCall(ctx, slotOf(movies.through, key, detailsCodec()), func(ctx context.Context) (Details, error) {
		return movies.inner.Details(ctx, request)
	})
}

// CachedShows is a ShowLookup that reads through the provider cache.
type CachedShows struct {
	inner   ShowLookup
	through readThrough
}

// NewCachedShows wraps the TMDB show lookup.
func NewCachedShows(inner ShowLookup, deps CacheDeps) *CachedShows {
	return &CachedShows{inner: inner, through: newReadThrough(deps, ProviderTMDB)}
}

// Search returns the cached or fetched candidates.
func (shows *CachedShows) Search(ctx context.Context, query SearchQuery) ([]match.Candidate, error) {
	key := cacheKey("show-search", query.Language, strconv.Itoa(query.Year), query.Title)

	return cachedCall(ctx, slotOf(shows.through, key, candidatesCodec()), func(ctx context.Context) ([]match.Candidate, error) {
		return shows.inner.Search(ctx, query)
	})
}

// Details returns the cached or fetched show.
func (shows *CachedShows) Details(ctx context.Context, request DetailsRequest) (Details, error) {
	key := cacheKey("show-details", refKey(request.Ref), request.Language)

	return cachedCall(ctx, slotOf(shows.through, key, detailsCodec()), func(ctx context.Context) (Details, error) {
		return shows.inner.Details(ctx, request)
	})
}

// SeasonDetails returns the cached or fetched season.
func (shows *CachedShows) SeasonDetails(ctx context.Context, request SeasonRequest) (SeasonDetails, error) {
	key := cacheKey("show-season", refKey(request.Show), strconv.Itoa(request.Season), request.Language)

	return cachedCall(ctx, slotOf(shows.through, key, seasonCodec()), func(ctx context.Context) (SeasonDetails, error) {
		return shows.inner.SeasonDetails(ctx, request)
	})
}

// CachedMusic is a MusicLookup that reads through the provider cache.
type CachedMusic struct {
	inner   MusicLookup
	through readThrough
}

// NewCachedMusic wraps the MusicBrainz lookup.
func NewCachedMusic(inner MusicLookup, deps CacheDeps) *CachedMusic {
	return &CachedMusic{inner: inner, through: newReadThrough(deps, ProviderMusicBrainz)}
}

// Search returns the cached or fetched candidates.
func (music *CachedMusic) Search(ctx context.Context, query MusicQuery) ([]match.Candidate, error) {
	key := cacheKey("music-search", string(query.Kind), strconv.Itoa(query.Year), query.Artist, query.Title)

	return cachedCall(ctx, slotOf(music.through, key, candidatesCodec()), func(ctx context.Context) ([]match.Candidate, error) {
		return music.inner.Search(ctx, query)
	})
}

// Details returns the cached or fetched release or artist.
func (music *CachedMusic) Details(ctx context.Context, request DetailsRequest) (Details, error) {
	key := cacheKey("music-details", refKey(request.Ref), request.Language)

	return cachedCall(ctx, slotOf(music.through, key, detailsCodec()), func(ctx context.Context) (Details, error) {
		return music.inner.Details(ctx, request)
	})
}

// ObservedArtwork reports the outcome of every picture download to the provider's health.
// Pictures are stored files, so there is nothing to cache.
type ObservedArtwork struct {
	inner  ArtworkDownloader
	health *Health
}

// NewObservedArtwork wraps the artwork downloader.
// health should be the instance the Queue reports; nil means a private one.
func NewObservedArtwork(inner ArtworkDownloader, health *Health) *ObservedArtwork {
	if health == nil {
		health = NewHealth(HealthOptions{})
	}

	return &ObservedArtwork{inner: inner, health: health}
}

// Fetch downloads the picture unless its provider is disabled.
func (observed *ObservedArtwork) Fetch(ctx context.Context, ref ArtworkRef) (Stored, error) {
	provider := ProviderName(ref.Provider)
	if !observed.health.Usable(provider) {
		return Stored{}, fmt.Errorf("fetch %s picture: %w", provider, errProviderDisabled)
	}

	stored, err := observed.inner.Fetch(ctx, ref)
	observed.health.Observe(provider, err)

	return stored, err
}
