package watch_test

import (
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/watch"
)

const movieSeconds = 6000 // 100 minutes, so 1 % is 60 s and 90 % is 5400 s

// now is the fixed report time of the tests; earlier is a previous playback.
func now() time.Time { return time.Date(2026, time.November, 6, 20, 0, 0, 0, time.UTC) }

func earlier() time.Time { return time.Date(2026, time.October, 1, 20, 0, 0, 0, time.UTC) }

func TestApplyProgress(t *testing.T) {
	t.Parallel()

	inProgress := watch.State{PositionSeconds: 1200, PlayCount: 1, LastPlayed: earlier()}
	watched := watch.State{Watched: true, PlayCount: 2, LastPlayed: earlier()}

	tests := []struct {
		name     string
		state    watch.State
		position float64
		duration float64
		want     watch.State
	}{
		{
			name: "short peek on a long movie keeps not started", position: 59, duration: movieSeconds,
			want: watch.State{},
		},
		{
			name: "1 % of a long movie is in progress", position: 60, duration: movieSeconds,
			want: watch.State{PositionSeconds: 60, LastPlayed: now()},
		},
		{
			name: "29 s on a short episode keeps not started", position: 29, duration: 1200,
			want: watch.State{},
		},
		{
			name: "30 s on a short episode is in progress", position: 30, duration: 1200,
			want: watch.State{PositionSeconds: 30, LastPlayed: now()},
		},
		{
			name: "89.9 % stays in progress", position: 5394, duration: movieSeconds,
			want: watch.State{PositionSeconds: 5394, LastPlayed: now()},
		},
		{
			name: "90 % marks watched", position: 5400, duration: movieSeconds,
			want: watch.State{Watched: true, PlayCount: 1, LastPlayed: now()},
		},
		{
			name: "position past the end marks watched", position: 7000, duration: movieSeconds,
			want: watch.State{Watched: true, PlayCount: 1, LastPlayed: now()},
		},
		{
			name: "finishing an in-progress item increments the play count", state: inProgress,
			position: 5900, duration: movieSeconds,
			want: watch.State{Watched: true, PlayCount: 2, LastPlayed: now()},
		},
		{
			name: "repeated report after completion does not count twice", state: watched,
			position: 5500, duration: movieSeconds,
			want: watch.State{Watched: true, PlayCount: 2, LastPlayed: now()},
		},
		{
			name: "rewatching a watched item stores the position", state: watched,
			position: 300, duration: movieSeconds,
			want: watch.State{Watched: true, PlayCount: 2, PositionSeconds: 300, LastPlayed: now()},
		},
		{
			name: "seeking back below the threshold resets to not started", state: inProgress,
			position: 10, duration: movieSeconds,
			want: watch.State{PlayCount: 1, LastPlayed: earlier()},
		},
		{
			name: "20 s file: 17 s keeps not started", position: 17, duration: 20,
			want: watch.State{},
		},
		{
			name: "20 s file: 18 s marks watched", position: 18, duration: 20,
			want: watch.State{Watched: true, PlayCount: 1, LastPlayed: now()},
		},
		{
			name: "unknown duration stores the position", position: 4000, duration: 0,
			want: watch.State{PositionSeconds: 4000, LastPlayed: now()},
		},
		{
			name: "unknown duration never marks watched", position: 1e6, duration: -1,
			want: watch.State{PositionSeconds: 1e6, LastPlayed: now()},
		},
		{
			name: "unknown duration still ignores a short peek", position: 29, duration: 0,
			want: watch.State{},
		},
		{
			name: "negative position counts as zero", position: -5, duration: movieSeconds,
			want: watch.State{},
		},
		{
			name: "a new position un-hides the item", state: watch.State{PositionSeconds: 900, Hidden: true, LastPlayed: earlier()},
			position: 1000, duration: movieSeconds,
			want: watch.State{PositionSeconds: 1000, LastPlayed: now()},
		},
		{
			name: "a short peek keeps a hidden item hidden", state: watch.State{Hidden: true},
			position: 5, duration: movieSeconds,
			want: watch.State{Hidden: true},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := watch.ApplyProgress(test.state, test.position, test.duration, now())
			if got != test.want {
				t.Errorf("ApplyProgress() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestMarkWatched(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state watch.State
		want  watch.State
	}{
		{
			name: "unwatched item becomes watched once",
			want: watch.State{Watched: true, PlayCount: 1, LastPlayed: now()},
		},
		{
			name:  "in-progress item clears its position",
			state: watch.State{PositionSeconds: 400, Hidden: true, LastPlayed: earlier()},
			want:  watch.State{Watched: true, PlayCount: 1, LastPlayed: now()},
		},
		{
			name:  "already watched item is unchanged",
			state: watch.State{Watched: true, PlayCount: 3, LastPlayed: earlier()},
			want:  watch.State{Watched: true, PlayCount: 3, LastPlayed: earlier()},
		},
		{
			name:  "watched item being rewatched finishes the rewatch",
			state: watch.State{Watched: true, PlayCount: 3, PositionSeconds: 700, LastPlayed: earlier()},
			want:  watch.State{Watched: true, PlayCount: 4, LastPlayed: now()},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := watch.MarkWatched(test.state, now()); got != test.want {
				t.Errorf("MarkWatched() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestMarkUnwatched(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state watch.State
		want  watch.State
	}{
		{name: "empty state stays empty", want: watch.State{}},
		{
			name:  "watched item keeps its play count",
			state: watch.State{Watched: true, PlayCount: 2, LastPlayed: earlier()},
			want:  watch.State{PlayCount: 2, LastPlayed: earlier()},
		},
		{
			name:  "in-progress item clears its position",
			state: watch.State{PositionSeconds: 300, Hidden: true, PlayCount: 1, LastPlayed: earlier()},
			want:  watch.State{Hidden: true, PlayCount: 1, LastPlayed: earlier()},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := watch.MarkUnwatched(test.state); got != test.want {
				t.Errorf("MarkUnwatched() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestInProgress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state watch.State
		want  bool
	}{
		{name: "not started", state: watch.State{}, want: false},
		{name: "with a position", state: watch.State{PositionSeconds: 30}, want: true},
		{name: "watched", state: watch.State{Watched: true, PlayCount: 1}, want: false},
		{name: "rewatch with a position", state: watch.State{Watched: true, PositionSeconds: 90}, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := test.state.InProgress(); got != test.want {
				t.Errorf("InProgress() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestInContinueWatching(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state watch.State
		want  bool
	}{
		{name: "recent in-progress item", state: watch.State{PositionSeconds: 300, LastPlayed: earlier()}, want: true},
		{
			name:  "played exactly at the window edge",
			state: watch.State{PositionSeconds: 300, LastPlayed: now().Add(-watch.ContinueWatchingWindow)},
			want:  true,
		},
		{
			name:  "played before the window",
			state: watch.State{PositionSeconds: 300, LastPlayed: now().Add(-watch.ContinueWatchingWindow - time.Second)},
			want:  false,
		},
		{name: "hidden item", state: watch.State{PositionSeconds: 300, Hidden: true, LastPlayed: earlier()}, want: false},
		{name: "not started", state: watch.State{LastPlayed: earlier()}, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := watch.InContinueWatching(test.state, now()); got != test.want {
				t.Errorf("InContinueWatching() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestSupportsWatchStateAndMarkTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind         library.ItemKind
		wantState    bool
		wantMarkable bool
	}{
		{kind: library.ItemKindMovie, wantState: true, wantMarkable: true},
		{kind: library.ItemKindEpisode, wantState: true, wantMarkable: true},
		{kind: library.ItemKindAudiobook, wantState: true, wantMarkable: true},
		{kind: library.ItemKindShow, wantState: false, wantMarkable: true},
		{kind: library.ItemKindSeason, wantState: false, wantMarkable: true},
		{kind: library.ItemKindExtra, wantState: false, wantMarkable: false},
		{kind: library.ItemKindArtist, wantState: false, wantMarkable: false},
		{kind: library.ItemKindAlbum, wantState: false, wantMarkable: false},
		{kind: library.ItemKindPhoto, wantState: false, wantMarkable: false},
		{kind: library.ItemKind("unknown"), wantState: false, wantMarkable: false},
	}

	for _, test := range tests {
		t.Run(string(test.kind), func(t *testing.T) {
			t.Parallel()

			if got := watch.SupportsWatchState(test.kind); got != test.wantState {
				t.Errorf("SupportsWatchState(%q) = %v, want %v", test.kind, got, test.wantState)
			}

			if got := watch.IsMarkTarget(test.kind); got != test.wantMarkable {
				t.Errorf("IsMarkTarget(%q) = %v, want %v", test.kind, got, test.wantMarkable)
			}
		})
	}
}
