package watch_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/access"
	appwatch "github.com/magpeek/mivo/internal/app/watch"
	"github.com/magpeek/mivo/internal/domain"
	domainaccess "github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
)

func TestNewServiceRequiresEveryDependency(t *testing.T) {
	t.Parallel()

	catalog := newFakeCatalog()
	complete := appwatch.Dependencies{Items: catalog, Episodes: catalog, Store: newFakeStore(catalog), Clock: time.Now}

	tests := []struct {
		name   string
		modify func(*appwatch.Dependencies)
	}{
		{"Items", func(deps *appwatch.Dependencies) { deps.Items = nil }},
		{"Episodes", func(deps *appwatch.Dependencies) { deps.Episodes = nil }},
		{"Store", func(deps *appwatch.Dependencies) { deps.Store = nil }},
		{"Clock", func(deps *appwatch.Dependencies) { deps.Clock = nil }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			deps := complete
			test.modify(&deps)

			if _, err := appwatch.NewService(deps); !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("NewService without %s: error = %v, want ErrInvalid", test.name, err)
			}
		})
	}

	if _, err := appwatch.NewService(complete); err != nil {
		t.Fatalf("NewService with all dependencies: %v", err)
	}
}

// TestEveryOperationNeedsAViewerWithAProfile covers the fail-closed rule: no viewer is
// unauthenticated, a viewer without a selected profile gets the profile error, and neither
// reaches the catalog or the store.
func TestEveryOperationNeedsAViewerWithAProfile(t *testing.T) {
	t.Parallel()

	contexts := []struct {
		name    string
		ctx     context.Context
		wantErr error
	}{
		{"no viewer", context.Background(), domain.ErrUnauthenticated},
		{"no profile", access.WithViewer(context.Background(), domainaccess.Viewer{AccountID: "account-1"}), access.ErrProfileRequired},
	}

	operations := []struct {
		name string
		run  func(context.Context, *appwatch.Service) error
	}{
		{"ReportProgress", func(ctx context.Context, service *appwatch.Service) error {
			return service.ReportProgress(ctx, "movie-1", 600)
		}},
		{"MarkWatched", func(ctx context.Context, service *appwatch.Service) error { return service.MarkWatched(ctx, "movie-1") }},
		{"MarkUnwatched", func(ctx context.Context, service *appwatch.Service) error {
			return service.MarkUnwatched(ctx, "movie-1")
		}},
		{"HideFromContinueWatching", func(ctx context.Context, service *appwatch.Service) error {
			return service.HideFromContinueWatching(ctx, "movie-1")
		}},
		{"States", func(ctx context.Context, service *appwatch.Service) error {
			_, err := service.States(ctx, []appwatch.ItemRef{{ID: "movie-1", Kind: library.ItemKindMovie}})

			return err
		}},
	}

	for _, viewerCase := range contexts {
		for _, operation := range operations {
			t.Run(viewerCase.name+"/"+operation.name, func(t *testing.T) {
				t.Parallel()

				fixture := newFixture(t)
				fixture.catalog.add(appwatch.Target{ItemID: "movie-1", Kind: library.ItemKindMovie, DurationSeconds: 6000})

				if err := operation.run(viewerCase.ctx, fixture.service); !errors.Is(err, viewerCase.wantErr) {
					t.Fatalf("error = %v, want %v", err, viewerCase.wantErr)
				}

				if fixture.catalog.lookups != 0 || fixture.store.updates != 0 || fixture.store.stateReads != 0 {
					t.Fatalf("lookups %d, updates %d, reads %d; want none", fixture.catalog.lookups, fixture.store.updates, fixture.store.stateReads)
				}
			})
		}
	}
}
