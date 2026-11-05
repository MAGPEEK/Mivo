package sqlite_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/domain"
)

const (
	testMetadataTTL = 7 * 24 * time.Hour
	testNotFoundTTL = 30 * 24 * time.Hour
)

func TestProviderCacheRoundTripsAndReplacesEntries(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	found := sqlite.CacheEntry{
		Payload:   []byte(`{"title":"The Film","year":2024}`),
		FetchedAt: enrichmentStart(),
		ExpiresAt: enrichmentStart().Add(testMetadataTTL),
	}
	notFound := sqlite.CacheEntry{
		NotFound:  true,
		FetchedAt: enrichmentStart().Add(time.Hour),
		ExpiresAt: enrichmentStart().Add(time.Hour + testNotFoundTTL),
	}

	for _, want := range []sqlite.CacheEntry{found, notFound, found} {
		if err := fixture.store.WriteProviderCache(t.Context(), "tmdb", "movie:603:en", want); err != nil {
			t.Fatalf("WriteProviderCache error = %v", err)
		}

		got, err := fixture.store.ReadProviderCache(t.Context(), "tmdb", "movie:603:en")
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("ReadProviderCache = %+v, %v, want %+v", got, err, want)
		}
	}

	if _, err := fixture.store.ReadProviderCache(t.Context(), "musicbrainz", "movie:603:en"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ReadProviderCache of another provider error = %v, want ErrNotFound", err)
	}
}

// Offline the stale answer is better than none (ADR 0031 §8): reads ignore the expiry.
func TestProviderCacheReturnsExpiredEntries(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	entry := sqlite.CacheEntry{Payload: []byte(`[]`), FetchedAt: enrichmentStart(), ExpiresAt: enrichmentStart().Add(time.Minute)}

	if err := fixture.store.WriteProviderCache(t.Context(), "tmdb", "search:film", entry); err != nil {
		t.Fatalf("WriteProviderCache error = %v", err)
	}

	got, err := fixture.store.ReadProviderCache(t.Context(), "tmdb", "search:film")
	if err != nil {
		t.Fatalf("ReadProviderCache error = %v", err)
	}

	if got.Expired(enrichmentStart()) || !got.Expired(enrichmentStart().Add(time.Minute)) {
		t.Errorf("Expired at start, at expiry = %t, %t, want false, true",
			got.Expired(enrichmentStart()), got.Expired(enrichmentStart().Add(time.Minute)))
	}
}

func TestProviderCacheRejectsWhatItCannotStore(t *testing.T) {
	t.Parallel()

	valid := sqlite.CacheEntry{Payload: []byte(`{}`), FetchedAt: enrichmentStart(), ExpiresAt: enrichmentStart()}
	tests := []struct {
		name          string
		provider, key string
		entry         sqlite.CacheEntry
	}{
		{"empty provider", "", "k", valid},
		{"empty key", "tmdb", "", valid},
		{"payload too large", "tmdb", "k", sqlite.CacheEntry{
			Payload: []byte(`"` + strings.Repeat("a", sqlite.MaxCachePayloadBytes) + `"`), ExpiresAt: enrichmentStart(),
		}},
		{"payload not JSON", "tmdb", "k", sqlite.CacheEntry{Payload: []byte(`{`), ExpiresAt: enrichmentStart()}},
		{"found without payload", "tmdb", "k", sqlite.CacheEntry{ExpiresAt: enrichmentStart()}},
		{"not found with payload", "tmdb", "k", sqlite.CacheEntry{NotFound: true, Payload: []byte(`{}`)}},
		{"expiry before the fetch", "tmdb", "k", sqlite.CacheEntry{
			Payload: []byte(`{}`), FetchedAt: enrichmentStart(), ExpiresAt: enrichmentStart().Add(-time.Second),
		}},
	}

	fixture := newEnrichmentFixture(t)

	for _, test := range tests {
		err := fixture.store.WriteProviderCache(t.Context(), test.provider, test.key, test.entry)
		if !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: WriteProviderCache error = %v, want ErrInvalid", test.name, err)
		}
	}

	if got := rowCount(t, fixture.inspection, "provider_cache"); got != 0 {
		t.Errorf("provider_cache rows = %d, want 0", got)
	}
}
