package rows_test

import (
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/rows"
)

func TestWatchRowOrder(t *testing.T) {
	t.Parallel()

	if got, want := rows.HomeLeadKinds(), []rows.Kind{rows.KindContinueWatching, rows.KindUpNext}; !slices.Equal(got, want) {
		t.Errorf("HomeLeadKinds = %v, want %v", got, want)
	}

	if got, want := rows.OverviewLeadKinds(), []rows.Kind{rows.KindContinueWatching}; !slices.Equal(got, want) {
		t.Errorf("OverviewLeadKinds = %v, want %v", got, want)
	}
}

func TestWatchRowLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		kind      rows.Kind
		requested int
		want      int
	}{
		{name: "continue watching below cap", kind: rows.KindContinueWatching, requested: 20, want: 20},
		{name: "continue watching capped", kind: rows.KindContinueWatching, requested: 50, want: 40},
		{name: "up next capped", kind: rows.KindUpNext, requested: 50, want: 20},
		{name: "up next below cap", kind: rows.KindUpNext, requested: 5, want: 5},
		{name: "other kind", kind: rows.KindRecentlyAdded, requested: 5, want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := rows.WatchRowLimit(test.kind, test.requested); got != test.want {
				t.Errorf("WatchRowLimit = %d, want %d", got, test.want)
			}
		})
	}
}

func TestWatchRowID(t *testing.T) {
	t.Parallel()

	if got := rows.WatchRowID(rows.KindUpNext, ""); got != "upNext" {
		t.Errorf("Home ID = %q, want upNext", got)
	}

	if got := rows.WatchRowID(rows.KindContinueWatching, "lib-1"); got != "continueWatching:lib-1" {
		t.Errorf("library ID = %q", got)
	}
}

func TestWatchRowsUsePosterCards(t *testing.T) {
	t.Parallel()

	for _, kind := range rows.HomeLeadKinds() {
		if got := rows.CardFormatOf(kind, library.KindShows); got != rows.CardPoster {
			t.Errorf("CardFormatOf(%s) = %s, want poster", kind, got)
		}
	}
}
