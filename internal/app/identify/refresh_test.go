package identify_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

func TestRefreshItemQueuesItemAndShowChildren(t *testing.T) {
	tests := []struct {
		kind library.ItemKind
		want []string
	}{
		{library.ItemKindMovie, []string{"item"}},
		{library.ItemKindEpisode, []string{"item"}},
		{library.ItemKindShow, []string{"item", "season-1", "episode-1"}},
	}

	for _, test := range tests {
		t.Run(string(test.kind), func(t *testing.T) {
			h := newHarness()
			h.addItem("item", test.kind, enrich.ItemSnapshot{})
			h.store.children["item"] = []string{"season-1", "episode-1"}

			if err := h.service().RefreshItem(context.Background(), "item"); err != nil {
				t.Fatalf("RefreshItem: %v", err)
			}

			if !slices.Equal(h.queue.itemIDs(), test.want) {
				t.Fatalf("queued = %v, want %v", h.queue.itemIDs(), test.want)
			}

			for _, call := range h.queue.calls {
				if call.reason != enrich.ReasonRefresh {
					t.Fatalf("reason = %s, want refresh", call.reason)
				}
			}
		})
	}
}

func TestRefreshItemErrors(t *testing.T) {
	tests := []struct {
		name   string
		change func(h *harness)
		want   error
	}{
		{"online off", func(h *harness) { h.settings.settings.Enabled = false }, domain.ErrConflict},
		{"library off", func(h *harness) {
			snapshot := h.store.items["item"]
			snapshot.OnlineEnabled = false
			h.store.items["item"] = snapshot
		}, domain.ErrConflict},
		{"unknown item", func(h *harness) { delete(h.store.items, "item") }, domain.ErrNotFound},
		{"queue fails", func(h *harness) { h.queue.err = errBoom }, errBoom},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness()
			h.addItem("item", library.ItemKindMovie, enrich.ItemSnapshot{})
			test.change(h)

			if err := h.service().RefreshItem(context.Background(), "item"); !errors.Is(err, test.want) {
				t.Fatalf("RefreshItem error = %v, want %v", err, test.want)
			}

			if len(h.queue.calls) != 0 {
				t.Fatal("a refused refresh queued an item")
			}
		})
	}
}

func TestRefreshLibraryQueuesMatchableItems(t *testing.T) {
	h := newHarness()
	h.store.libraries["lib"] = true
	h.store.matchable["lib"] = []string{"a", "gone", "b"}
	h.queue.missing["gone"] = true

	if err := h.service().RefreshLibrary(context.Background(), "lib"); err != nil {
		t.Fatalf("RefreshLibrary: %v", err)
	}

	if want := []string{"a", "b"}; !slices.Equal(h.queue.itemIDs(), want) {
		t.Fatalf("queued = %v, want %v", h.queue.itemIDs(), want)
	}
}

func TestRefreshLibraryErrors(t *testing.T) {
	tests := []struct {
		name   string
		change func(h *harness)
		want   error
	}{
		{"online off", func(h *harness) { h.settings.settings.Enabled = false }, domain.ErrConflict},
		{"library off", func(h *harness) { h.store.libraries["lib"] = false }, domain.ErrConflict},
		{"unknown library", func(h *harness) { delete(h.store.libraries, "lib") }, domain.ErrNotFound},
		{"settings fail", func(h *harness) { h.settings.err = errBoom }, errBoom},
		{"listing fails", func(h *harness) { h.store.listErr = errBoom }, errBoom},
		{"queue fails", func(h *harness) { h.queue.err = errBoom }, errBoom},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness()
			h.store.libraries["lib"] = true
			h.store.matchable["lib"] = []string{"a"}
			test.change(h)

			if err := h.service().RefreshLibrary(context.Background(), "lib"); !errors.Is(err, test.want) {
				t.Fatalf("RefreshLibrary error = %v, want %v", err, test.want)
			}
		})
	}
}
