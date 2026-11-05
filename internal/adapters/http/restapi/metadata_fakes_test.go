package restapi_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/app/identify"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

// appliedMatch is one recorded Apply call.
type appliedMatch struct {
	itemID     string
	provider   match.RefProvider
	externalID string
}

// fakeIdentify is an in-memory IdentifyService and MetadataRefresher.
type fakeIdentify struct {
	mutex sync.Mutex

	found []match.Scored
	err   error

	queries   []identify.Query
	applied   []appliedMatch
	unmatched []string
	refreshed []string
}

func (fake *fakeIdentify) Search(_ context.Context, _ string, query identify.Query) ([]match.Scored, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	fake.queries = append(fake.queries, query)

	return fake.found, fake.err
}

func (fake *fakeIdentify) Apply(_ context.Context, itemID string, provider match.RefProvider, externalID string) error {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	fake.applied = append(fake.applied, appliedMatch{itemID: itemID, provider: provider, externalID: externalID})

	return fake.err
}

func (fake *fakeIdentify) Unmatch(_ context.Context, itemID string) error {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	fake.unmatched = append(fake.unmatched, itemID)

	return fake.err
}

func (fake *fakeIdentify) RefreshItem(_ context.Context, itemID string) error {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	fake.refreshed = append(fake.refreshed, "item:"+itemID)

	return fake.err
}

func (fake *fakeIdentify) RefreshLibrary(_ context.Context, libraryID string) error {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	fake.refreshed = append(fake.refreshed, "library:"+libraryID)

	return fake.err
}

// fakeStatus is an in-memory MetadataStatusSource.
type fakeStatus struct {
	status enrich.Status
	err    error
}

func (fake fakeStatus) Status(context.Context) (enrich.Status, error) { return fake.status, fake.err }

// fakeMatches is an in-memory MatchStatusReader.
type fakeMatches struct {
	statuses map[string]match.ItemStatus
	err      error
}

func (fake fakeMatches) ItemMatchStatuses(_ context.Context, itemIDs []string) (map[string]match.ItemStatus, error) {
	if fake.err != nil {
		return nil, fake.err
	}

	found := map[string]match.ItemStatus{}
	for _, itemID := range itemIDs {
		if status, ok := fake.statuses[itemID]; ok {
			found[itemID] = status
		}
	}

	return found, nil
}

// fakePosters hands out proxy URLs by path and records the registered references.
type fakePosters struct {
	mutex      sync.Mutex
	registered []enrich.ArtworkRef
}

func (fake *fakePosters) PosterURL(ref enrich.ArtworkRef) string {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	fake.registered = append(fake.registered, ref)

	return "/api/v1/metadata/posters/" + strings.Repeat("a", 32)
}

// rateLimited is a rate-limit error that names its pause, like enrich.RetryAfterError.
type rateLimited struct{ pause time.Duration }

func (err rateLimited) Error() string             { return "rate limited" }
func (err rateLimited) RetryAfter() time.Duration { return err.pause }
func (err rateLimited) Unwrap() error             { return domain.ErrRateLimited }

// providerError marks err as a failed provider call, as identify.Service.Search does.
func providerError(err error) error {
	return fmt.Errorf("search movie: %w: %w", identify.ErrProvider, err)
}

func newMetadataHandler(t *testing.T, items *fakeLibraryService, services restapi.MetadataServices) (http.Handler, *syncBuffer) {
	t.Helper()

	if items == nil {
		items = &fakeLibraryService{}
	}

	return newTestHandlerWithOptions(t, restapi.Options{Libraries: items, Items: items, Metadata: services})
}
