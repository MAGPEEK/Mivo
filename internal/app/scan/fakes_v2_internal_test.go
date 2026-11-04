package scan_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

func movieChain(key string) library.Placement {
	return library.Placement{Levels: []library.Level{{Kind: library.ItemKindMovie, MatchKey: key, Title: key}}}
}

func showChain(episode string) library.Placement {
	return library.Placement{Levels: []library.Level{
		{Kind: library.ItemKindShow, MatchKey: "show", Title: "Show"},
		{Kind: library.ItemKindSeason, MatchKey: "1", Title: "Season 1", Number: library.NumberOf(1)},
		{Kind: library.ItemKindEpisode, MatchKey: episode, Title: episode, Number: library.NumberOf(1)},
	}}
}

// TestMemoryCatalogV2Contract table-tests the rules the fake enforces for the sqlite writer.
//
//nolint:gocognit // a table of small closures, one per contract rule
func TestMemoryCatalogV2Contract(t *testing.T) {
	t.Parallel()

	save := func(catalog *memoryCatalogV2, fileID, path string, placement library.Placement) ([]string, error) {
		next := 0
		newID := func() (string, error) { next++; return fileID + "-item-" + strconv.Itoa(next), nil }

		return catalog.SavePlacedFile(context.Background(), scan.PlacedFile{
			Placement: placement, ProbedAt: time.Unix(1, 0),
			File: media.File{ID: fileID, RelativePath: path},
		}, newID)
	}

	tests := []struct {
		name string
		run  func(t *testing.T, catalog *memoryCatalogV2)
	}{
		{"siblings share the chain, items keep their IDs", func(t *testing.T, catalog *memoryCatalogV2) {
			t.Helper()

			first, _ := save(catalog, "f1", "a", showChain("e1"))
			second, _ := save(catalog, "f2", "b", showChain("e2"))
			again, _ := save(catalog, "f1", "a", showChain("e1"))

			if first[0] != second[0] || first[1] != second[1] || first[2] == second[2] || again[2] != first[2] {
				t.Errorf("chains = %v %v %v", first, second, again)
			}
		}},
		{"removal prunes emptied ancestors bottom-up", func(t *testing.T, catalog *memoryCatalogV2) {
			t.Helper()

			_, _ = save(catalog, "f1", "a", showChain("e1"))
			_, _ = save(catalog, "f2", "b", showChain("e2"))
			_ = catalog.RemovePlacedFiles(context.Background(), "", []string{"f1", "unknown"})

			if catalog.count(library.ItemKindEpisode) != 1 || catalog.count(library.ItemKindShow) != 1 {
				t.Error("removing one episode must keep the shared show")
			}

			_ = catalog.RemovePlacedFiles(context.Background(), "", []string{"f2"})

			if len(catalog.items) != 0 {
				t.Errorf("items left = %d, want 0", len(catalog.items))
			}
		}},
		{"a path belongs to one file id", func(t *testing.T, catalog *memoryCatalogV2) {
			t.Helper()

			_, _ = save(catalog, "f1", "a", movieChain("m"))

			if _, err := save(catalog, "f2", "a", movieChain("m")); !errors.Is(err, domain.ErrConflict) {
				t.Errorf("error = %v, want ErrConflict", err)
			}
		}},
		{"an invalid placement or a zero probe time is refused", func(t *testing.T, catalog *memoryCatalogV2) {
			t.Helper()

			if _, err := save(catalog, "f1", "a", library.Placement{}); !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("empty placement error = %v, want ErrInvalid", err)
			}

			_, err := catalog.SavePlacedFile(context.Background(),
				scan.PlacedFile{Placement: movieChain("m"), File: media.File{ID: "f"}}, nil)
			if !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("zero probe time error = %v, want ErrInvalid", err)
			}
		}},
		{"a move prunes the old chain and unknown files are not found", func(t *testing.T, catalog *memoryCatalogV2) {
			t.Helper()

			_, _ = save(catalog, "f1", "a", showChain("e1"))
			_, err := catalog.MoveFile(context.Background(), scan.MoveRequest{
				FileID: "f1", Placement: movieChain("m"), NewID: func() (string, error) { return "m1", nil },
			})

			if err != nil || catalog.count(library.ItemKindShow) != 0 || catalog.count(library.ItemKindMovie) != 1 {
				t.Errorf("move error = %v, shows = %d", err, catalog.count(library.ItemKindShow))
			}

			if _, err := catalog.MoveFile(context.Background(), scan.MoveRequest{FileID: "none", Placement: movieChain("m")}); !errors.Is(err, domain.ErrNotFound) {
				t.Errorf("unknown file error = %v, want ErrNotFound", err)
			}
		}},
		{"sidecar writes are idempotent", func(t *testing.T, catalog *memoryCatalogV2) {
			t.Helper()

			state := library.FileState{RelativePath: "a.nfo", Class: library.FileClassNFO}
			for range 2 {
				_ = catalog.SaveSidecarStates(context.Background(), "", []library.FileState{state}, []string{"gone.nfo"})
			}

			stored, _ := catalog.SidecarStates(context.Background(), "")
			if len(stored) != 1 {
				t.Errorf("sidecars = %d, want 1", len(stored))
			}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			test.run(t, newMemoryCatalogV2())
		})
	}
}
