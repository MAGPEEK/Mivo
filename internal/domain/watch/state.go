package watch

import (
	"time"

	"github.com/magpeek/mivo/internal/domain/library"
)

// WatchedThreshold is the fraction of the duration at which an item becomes watched (AD-23).
const WatchedThreshold = 0.9

// MinimumProgressSeconds and MinimumProgressFraction set the start of "in progress": a
// position below max(30 s, 1 % of the duration) is a peek, not a start, so browsing through
// items does not fill Continue watching.
const (
	MinimumProgressSeconds  = 30
	MinimumProgressFraction = 0.01
)

// Row bounds of Continue watching (HO-03) and Up next (HO-04).
const (
	ContinueWatchingWindow = 12 * 7 * 24 * time.Hour
	ContinueWatchingLimit  = 40
	UpNextLimit            = 20
)

// State is one profile's watch state of one item. The zero value is "not started".
type State struct {
	// PositionSeconds is the resume position; 0 means no position (not started or finished).
	PositionSeconds float64
	Watched         bool
	PlayCount       int
	// LastPlayed is the time of the last counted playback or watched mark; zero when never.
	LastPlayed time.Time
	// Hidden removes the item from Continue watching until a new position is reported.
	Hidden bool
}

// InProgress reports whether the state has a resume position. A watched item being
// rewatched is in progress too.
func (state State) InProgress() bool {
	return state.PositionSeconds > 0
}

// ApplyProgress returns the state after the client reported position (seconds) at now.
// duration is the item's duration from the catalog in seconds; 0 or less means unknown.
//
//   - Below max(30 s, 1 % of the duration) the item is not started: the position is cleared
//     and nothing else changes.
//   - At or above 90 % of a known duration the item becomes watched with no position. The play
//     count increments unless the state already is a finished watch (watched, no position), so
//     the repeated reports while the credits play count once. A rewatch counts again once it
//     has been in progress.
//   - Otherwise the position is stored, the item is played now and no longer hidden. With an
//     unknown duration this is all that happens; it never becomes watched automatically.
func ApplyProgress(state State, position, duration float64, now time.Time) State {
	position = max(position, 0)

	if isFinished(position, duration) {
		return finish(state, now)
	}

	if position < minimumProgress(duration) {
		state.PositionSeconds = 0

		return state
	}

	state.PositionSeconds = position
	state.LastPlayed = now
	state.Hidden = false

	return state
}

// MarkWatched returns the state after the user marked the item watched at now. Marking an
// already finished item changes nothing, so the play count is not inflated.
func MarkWatched(state State, now time.Time) State {
	if state.Watched && !state.InProgress() {
		return state
	}

	return finish(state, now)
}

// MarkUnwatched returns the state after the user marked the item unwatched: not watched and no
// position. The play count and last played time are history and stay.
func MarkUnwatched(state State) State {
	state.Watched = false
	state.PositionSeconds = 0

	return state
}

// InContinueWatching reports whether the item belongs in Continue watching at now: in
// progress, not hidden and played within ContinueWatchingWindow.
func InContinueWatching(state State, now time.Time) bool {
	return state.InProgress() && !state.Hidden && !state.LastPlayed.Before(now.Add(-ContinueWatchingWindow))
}

// SupportsWatchState reports whether items of the kind carry their own watch state.
func SupportsWatchState(kind library.ItemKind) bool {
	return kind == library.ItemKindMovie || kind == library.ItemKindEpisode || kind == library.ItemKindAudiobook
}

// IsMarkTarget reports whether items of the kind can be marked (un)watched. Shows and seasons
// have no state of their own; marking them applies to their episodes.
func IsMarkTarget(kind library.ItemKind) bool {
	return SupportsWatchState(kind) || kind == library.ItemKindShow || kind == library.ItemKindSeason
}

func isFinished(position, duration float64) bool {
	return duration > 0 && position >= WatchedThreshold*duration
}

func minimumProgress(duration float64) float64 {
	return max(MinimumProgressSeconds, MinimumProgressFraction*max(duration, 0))
}

// finish marks the state watched with no position. The play count increments unless the
// state already was a finished watch.
func finish(state State, now time.Time) State {
	if !state.Watched || state.InProgress() {
		state.PlayCount++
	}

	state.Watched = true
	state.PositionSeconds = 0
	state.LastPlayed = now
	state.Hidden = false

	return state
}
