package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
)

// CacheEntry is one cached provider answer (enrich.CacheEntry): provider-neutral JSON, or
// NotFound with no payload. An expired entry is still returned by ReadProviderCache; it is
// the offline fallback (ADR 0031 §8). The alias lets the store implement enrich.ProviderCache.
type CacheEntry = enrich.CacheEntry

// ReadProviderCache returns the cached answer of provider for key, expired or not. The error
// wraps domain.ErrNotFound when nothing is cached.
func (store *EnrichmentStore) ReadProviderCache(ctx context.Context, provider, key string) (CacheEntry, error) {
	row, err := sqlcgen.New(store.reader).GetProviderCache(ctx, sqlcgen.GetProviderCacheParams{
		Provider: provider, CacheKey: key,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return CacheEntry{}, fmt.Errorf("read %s cache entry %q: %w", provider, key, domain.ErrNotFound)
	}

	if err != nil {
		return CacheEntry{}, fmt.Errorf("read %s cache entry %q: %w", provider, key, err)
	}

	entry := CacheEntry{
		NotFound:  row.NotFound == storedTrue,
		FetchedAt: time.UnixMilli(row.FetchedAt).UTC(),
		ExpiresAt: time.UnixMilli(row.ExpiresAt).UTC(),
	}
	if row.Payload != "" {
		entry.Payload = []byte(row.Payload)
	}

	return entry, nil
}

// WriteProviderCache stores or replaces the cached answer of provider for key. The error
// wraps domain.ErrInvalid for an empty provider or key, a payload that is not JSON or larger
// than MaxCachePayloadBytes, a payload on a not-found entry, a missing payload otherwise, or
// an expiry before the fetch.
func (store *EnrichmentStore) WriteProviderCache(ctx context.Context, provider, key string, entry CacheEntry) error {
	if err := validateCacheEntry(provider, key, entry); err != nil {
		return fmt.Errorf("write %s cache entry %q: %w", provider, key, err)
	}

	err := sqlcgen.New(store.writer).UpsertProviderCache(ctx, sqlcgen.UpsertProviderCacheParams{
		Provider:  provider,
		CacheKey:  key,
		Payload:   string(entry.Payload),
		FetchedAt: entry.FetchedAt.UnixMilli(),
		ExpiresAt: entry.ExpiresAt.UnixMilli(),
		NotFound:  storedFlag(entry.NotFound),
	})
	if err != nil {
		return fmt.Errorf("write %s cache entry %q: %w", provider, key, err)
	}

	return nil
}

// MaxCachePayloadBytes bounds one cached payload. Provider-neutral records are a few KiB;
// the bound keeps a misbehaving adapter from growing the database without limit.
const MaxCachePayloadBytes = 1 << 20

func validateCacheEntry(provider, key string, entry CacheEntry) error {
	switch {
	case provider == "" || key == "":
		return fmt.Errorf("empty provider or cache key: %w", domain.ErrInvalid)
	case len(entry.Payload) > MaxCachePayloadBytes:
		return fmt.Errorf("payload of %d bytes exceeds %d: %w", len(entry.Payload), MaxCachePayloadBytes, domain.ErrInvalid)
	case entry.NotFound && len(entry.Payload) > 0:
		return fmt.Errorf("not-found entry with a payload: %w", domain.ErrInvalid)
	case !entry.NotFound && !json.Valid(entry.Payload):
		return fmt.Errorf("payload is not JSON: %w", domain.ErrInvalid)
	case entry.ExpiresAt.UnixMilli() < entry.FetchedAt.UnixMilli():
		return fmt.Errorf("expiry before the fetch: %w", domain.ErrInvalid)
	default:
		return nil
	}
}
