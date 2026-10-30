package library_test

import (
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// IDs handed out by the test generator, in order, and IDs of stored entities.
const (
	firstGeneratedID  = "11111111-1111-4111-8111-111111111111"
	secondGeneratedID = "22222222-2222-4222-8222-222222222222"
	moviesID          = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	showsID           = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	unknownID         = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
)

// Roots as the configuration produces them; the service treats them as opaque.
const (
	moviesRoot = "/media/movies"
	showsRoot  = "/media/shows"
	animeRoot  = "/media/anime"
)

// Root check timeouts of the tests: hung-check tests use the short one to stay fast; every
// other test uses the generous one, so a loaded machine running -race cannot make a check
// that answers at once look timed out.
const (
	testRootCheckTimeout     = 20 * time.Millisecond
	generousRootCheckTimeout = time.Minute
)

var (
	errBoom         = errors.New("boom")
	errIDsExhausted = errors.New("test id generator exhausted")
)

// testEnvironment bundles the fakes behind one Service.
type testEnvironment struct {
	store     *memoryLibraryStore
	items     *memoryItemReader
	generator *sequenceIDGenerator
	roots     applibrary.RootChecker
	timeout   time.Duration
	logger    *slog.Logger
	logs      *recordingHandler
}

func newTestEnvironment() *testEnvironment {
	logger, logs := newRecordingLogger()

	return &testEnvironment{
		store:     &memoryLibraryStore{},
		items:     newMemoryItemReader(),
		generator: &sequenceIDGenerator{ids: []string{firstGeneratedID, secondGeneratedID}},
		roots:     availableRoots(moviesRoot, showsRoot, animeRoot),
		timeout:   generousRootCheckTimeout,
		logger:    logger,
		logs:      logs,
	}
}

func (environment *testEnvironment) service(t *testing.T) *applibrary.Service {
	t.Helper()

	service, err := applibrary.NewService(applibrary.Dependencies{
		Libraries:        environment.store,
		Items:            environment.items,
		IDs:              environment.generator,
		Roots:            environment.roots,
		Logger:           environment.logger,
		RootCheckTimeout: environment.timeout,
	})
	if err != nil {
		t.Fatalf("NewService error = %v", err)
	}

	return service
}

// storedMoviesAndShows puts two libraries into the store, ordered by name.
func (environment *testEnvironment) storedMoviesAndShows() (movies, shows library.Summary) {
	movies = library.Summary{
		Library:   library.Library{ID: moviesID, Name: "Movies", Root: moviesRoot, Kind: library.KindMovies},
		ItemCount: 3,
	}
	shows = library.Summary{
		Library: library.Library{ID: showsID, Name: "Shows", Root: showsRoot, Kind: library.KindShows},
	}
	environment.store.put(movies, shows)

	return movies, shows
}

func TestNewServiceRejectsMissingDependencies(t *testing.T) {
	t.Parallel()

	complete := func() applibrary.Dependencies {
		logger, _ := newRecordingLogger()

		return applibrary.Dependencies{
			Libraries: &memoryLibraryStore{},
			Items:     newMemoryItemReader(),
			IDs:       &sequenceIDGenerator{},
			Roots:     availableRoots(),
			Logger:    logger,
		}
	}

	tests := []struct {
		name     string
		mutate   func(*applibrary.Dependencies)
		wantText string
	}{
		{name: "library store", mutate: func(d *applibrary.Dependencies) { d.Libraries = nil }, wantText: "library store"},
		{name: "item reader", mutate: func(d *applibrary.Dependencies) { d.Items = nil }, wantText: "item reader"},
		{name: "id generator", mutate: func(d *applibrary.Dependencies) { d.IDs = nil }, wantText: "id generator"},
		{name: "root checker", mutate: func(d *applibrary.Dependencies) { d.Roots = nil }, wantText: "root checker"},
		{name: "logger", mutate: func(d *applibrary.Dependencies) { d.Logger = nil }, wantText: "logger"},
		{
			name:     "negative root check timeout",
			mutate:   func(d *applibrary.Dependencies) { d.RootCheckTimeout = -time.Second },
			wantText: "root check timeout",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			dependencies := complete()
			test.mutate(&dependencies)

			service, err := applibrary.NewService(dependencies)
			assertWrappedError(t, err, domain.ErrInvalid, test.wantText)

			if service != nil {
				t.Error("NewService returned a service together with an error")
			}
		})
	}
}

func TestNewServiceReportsEveryMissingDependency(t *testing.T) {
	t.Parallel()

	_, err := applibrary.NewService(applibrary.Dependencies{})
	if err == nil {
		t.Fatal("NewService(empty) error = nil, want error")
	}

	for _, name := range []string{"library store", "item reader", "id generator", "root checker", "logger"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("NewService(empty) error = %q, want it to name %q", err, name)
		}
	}
}

// A zero timeout selects the default, so the wiring does not have to know it.
func TestNewServiceAcceptsAZeroRootCheckTimeout(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment()
	environment.timeout = 0
	movies, _ := environment.storedMoviesAndShows()

	overview, err := environment.service(t).GetLibrary(t.Context(), movies.Library.ID)
	if err != nil {
		t.Fatalf("GetLibrary error = %v", err)
	}

	if !overview.Available {
		t.Error("GetLibrary Available = false, want true with the default timeout")
	}
}

func assertWrappedError(t *testing.T, err, wantErr error, wantText string) {
	t.Helper()

	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want it to wrap %v", err, wantErr)
	}

	if !strings.Contains(err.Error(), wantText) {
		t.Errorf("error = %q, want context %q", err, wantText)
	}
}
