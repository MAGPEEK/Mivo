package playback_test

import (
	"errors"
	"sync"
	"testing"
	"testing/synctest"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain"
)

// startResult is the outcome of one concurrent Start.
type startResult struct {
	session appplayback.Session
	err     error
}

func TestConcurrentStartsNeverExceedTheTranscodeLimit(t *testing.T) {
	t.Parallel()

	const callers = 20

	env := newEnvironment()
	env.producer.gate = make(chan struct{})
	env.producer.entered = make(chan struct{}, callers)
	service := env.service(t)
	results := make(chan startResult, callers)

	for range callers {
		go func() {
			session, err := service.Start(t.Context(), request(transcodeFileID))
			results <- startResult{session: session, err: err}
		}()
	}

	// The callers that won a slot wait inside the producer; all others must be rejected
	// while those starts are still in flight.
	for range maxTranscodes {
		<-env.producer.entered
	}

	for range callers - maxTranscodes {
		if result := <-results; !errors.Is(result.err, appplayback.ErrSessionLimitReached) {
			t.Errorf("rejected Start = %+v, %v; want ErrSessionLimitReached", result.session, result.err)
		}
	}

	close(env.producer.gate)

	for range maxTranscodes {
		if result := <-results; result.err != nil {
			t.Errorf("admitted Start error = %v", result.err)
		}
	}

	if jobs := env.producer.jobs(); len(jobs) != maxTranscodes {
		t.Errorf("producer jobs = %d, want %d", len(jobs), maxTranscodes)
	}
}

func TestConcurrentRemuxStartsStopAtTheFFmpegCap(t *testing.T) {
	t.Parallel()

	const callers = 3 * cappedFFmpegSessions

	env := newEnvironment()
	service := env.service(t)
	errs := make([]error, callers)

	var group sync.WaitGroup
	for caller := range callers {
		group.Go(func() {
			_, errs[caller] = service.Start(t.Context(), request(remuxFileID))
		})
	}

	group.Wait()

	admitted := 0

	for _, err := range errs {
		switch {
		case err == nil:
			admitted++
		case !errors.Is(err, appplayback.ErrSessionLimitReached):
			t.Errorf("Start error = %v, want nil or ErrSessionLimitReached", err)
		}
	}

	if admitted != cappedFFmpegSessions {
		t.Errorf("admitted remux sessions = %d, want %d", admitted, cappedFFmpegSessions)
	}
}

func TestAStartingSessionIsInvisibleButHoldsItsSlot(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	env.options.MaxTranscodeSessions = 1
	env.producer.gate = make(chan struct{})
	env.producer.entered = make(chan struct{}, 1)
	service := env.service(t)
	results := make(chan startResult, 1)

	go func() {
		session, err := service.Start(t.Context(), request(transcodeFileID))
		results <- startResult{session: session, err: err}
	}()

	<-env.producer.entered

	pendingID := sessionID(1)
	if _, err := service.Get(t.Context(), pendingID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get of a starting session error = %v, want ErrNotFound", err)
	}

	if _, err := service.Access(t.Context(), pendingID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Access of a starting session error = %v, want ErrNotFound", err)
	}

	if err := service.Stop(t.Context(), pendingID); err != nil {
		t.Errorf("Stop of a starting session error = %v, want nil (unknown)", err)
	}

	if _, err := service.Start(t.Context(), request(transcodeFileID)); !errors.Is(err, appplayback.ErrSessionLimitReached) {
		t.Errorf("second transcode while the first starts: error = %v, want ErrSessionLimitReached", err)
	}

	close(env.producer.gate)

	result := <-results
	if result.err != nil || result.session.ID != pendingID {
		t.Fatalf("Start = %+v, %v; want session %s", result.session, result.err, pendingID)
	}

	if _, err := service.Get(t.Context(), pendingID); err != nil {
		t.Errorf("Get after the start error = %v", err)
	}
}

// startReplacementHeldAtTheGate starts a transcode replacing oldID and returns once its
// production waits at the producer gate.
func startReplacementHeldAtTheGate(t *testing.T, env *environment, service *appplayback.Service, oldID string) <-chan startResult {
	t.Helper()

	seek := request(transcodeFileID)
	seek.ReplacesSessionID = oldID

	return startHeldAtTheGate(t, env, service, seek)
}

// startHeldAtTheGate starts startRequest and returns once its production waits at the
// producer gate.
func startHeldAtTheGate(t *testing.T, env *environment, service *appplayback.Service, startRequest appplayback.StartRequest) <-chan startResult {
	t.Helper()

	env.producer.gate = make(chan struct{})
	env.producer.entered = make(chan struct{}, 1)
	results := make(chan startResult, 1)

	go func() {
		session, err := service.Start(t.Context(), startRequest)
		results <- startResult{session: session, err: err}
	}()

	<-env.producer.entered

	return results
}

func TestAccessOfTheOldSessionDuringASwapServesItsWorkspace(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	service := env.service(t)
	old := mustStart(t, service, request(remuxFileID))
	results := startReplacementHeldAtTheGate(t, env, service, old.ID)

	access, err := service.Access(t.Context(), old.ID)
	if err != nil {
		t.Fatalf("Access of the old session during the swap error = %v", err)
	}

	defer access.Release()

	if access.Directory != workspaceDir(old.ID) {
		t.Errorf("Access directory = %q during the swap, want the old workspace %q", access.Directory, workspaceDir(old.ID))
	}

	if err := access.WaitReady(t.Context()); err != nil {
		t.Errorf("WaitReady of the old session during the swap error = %v", err)
	}

	close(env.producer.gate)

	if result := <-results; result.err != nil {
		t.Fatalf("replacement error = %v", result.err)
	}
}

func TestALowerClassReplacementKeepsTheOldSessionsSlotCounted(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		env.options.MaxTranscodeSessions = 1
		service := env.service(t)
		old := mustStart(t, service, request(transcodeFileID))

		// A seek that turns the transcode into a remux: the new session re-encodes nothing,
		// but the old one keeps re-encoding until the remux runs.
		seek := request(remuxFileID)
		seek.ReplacesSessionID = old.ID
		results := startHeldAtTheGate(t, env, service, seek)
		env.producer.entered = make(chan struct{}, 1)
		third := make(chan error, 1)

		go func() {
			_, err := service.Start(t.Context(), request(audioTranscodeFileID))
			third <- err
		}()

		// An admitted third transcode would wait at the producer gate instead of answering.
		synctest.Wait()

		select {
		case err := <-third:
			if !errors.Is(err, appplayback.ErrSessionLimitReached) {
				t.Errorf("third transcode during the swap error = %v, want ErrSessionLimitReached", err)
			}
		default:
			t.Errorf("third transcode during the swap was admitted, want ErrSessionLimitReached")
		}

		env.producer.err = errBoom
		close(env.producer.gate)

		if result := <-results; !errors.Is(result.err, errBoom) {
			t.Fatalf("replacement = %+v, %v; want errBoom", result.session, result.err)
		}

		// The failed remux released its claim; the old transcode still runs and holds the slot.
		mustGet(t, service, old.ID)

		env.producer.err = nil

		if _, err := service.Start(t.Context(), request(audioTranscodeFileID)); !errors.Is(err, appplayback.ErrSessionLimitReached) {
			t.Errorf("transcode after the failed swap error = %v, want ErrSessionLimitReached", err)
		}
	})
}

func TestAHigherClassReplacementTakesOverTheOldFFmpegSlot(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	env.options.MaxTranscodeSessions = 1
	service := env.service(t)
	old := mustStart(t, service, request(remuxFileID))

	for range cappedFFmpegSessions - 1 {
		mustStart(t, service, request(remuxFileID))
	}

	// The FFmpeg cap is full; a remux turning into a transcode still takes over its slot.
	seek := request(transcodeFileID)
	seek.ReplacesSessionID = old.ID

	mustStart(t, service, seek)
}

func TestAReplacementHoldsTheSlotAtomicallyWhileItStarts(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	env.options.MaxTranscodeSessions = 1
	service := env.service(t)
	old := mustStart(t, service, request(transcodeFileID))
	results := startReplacementHeldAtTheGate(t, env, service, old.ID)

	// The viewer keeps watching the old session until the new production runs.
	if _, err := service.Get(t.Context(), old.ID); err != nil {
		t.Errorf("Get of the old session while its replacement starts error = %v", err)
	}

	if stops := env.producer.productionOf(old.ID).stopCount(); stops != 0 {
		t.Errorf("old production stops = %d while the replacement starts, want 0", stops)
	}

	// A third client cannot take the slot of the seeking viewer, nor can a second
	// replacement of the same session.
	if _, err := service.Start(t.Context(), request(audioTranscodeFileID)); !errors.Is(err, appplayback.ErrSessionLimitReached) {
		t.Errorf("third transcode during the seek error = %v, want ErrSessionLimitReached", err)
	}

	again := request(audioTranscodeFileID)
	again.ReplacesSessionID = old.ID

	if _, err := service.Start(t.Context(), again); !errors.Is(err, appplayback.ErrSessionLimitReached) {
		t.Errorf("second replacement of the same session error = %v, want ErrSessionLimitReached", err)
	}

	close(env.producer.gate)

	result := <-results
	if result.err != nil {
		t.Fatalf("replacement error = %v", result.err)
	}

	if stops := env.producer.productionOf(old.ID).stopCount(); stops != 1 {
		t.Errorf("old production stops = %d after the replacement started, want 1", stops)
	}

	if removed := env.workspace.removedIDs(); len(removed) != 1 || removed[0] != old.ID {
		t.Errorf("removed workspaces = %v, want [%s]", removed, old.ID)
	}

	if _, err := service.Get(t.Context(), old.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get of the replaced session error = %v, want ErrNotFound", err)
	}
}

func TestASessionBeingReplacedCountsOnlyOnce(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	service := env.service(t)
	old := mustStart(t, service, request(transcodeFileID))
	replacement := startReplacementHeldAtTheGate(t, env, service, old.ID)
	env.producer.entered = make(chan struct{}, 1)
	other := make(chan startResult, 1)

	// Old session plus its pending replacement occupy one of the two re-encoding slots, so
	// another viewer still gets the second one.
	go func() {
		session, err := service.Start(t.Context(), request(audioTranscodeFileID))
		other <- startResult{session: session, err: err}
	}()

	<-env.producer.entered

	if _, err := service.Start(t.Context(), request(transcodeFileID)); !errors.Is(err, appplayback.ErrSessionLimitReached) {
		t.Errorf("third viewer error = %v, want ErrSessionLimitReached", err)
	}

	close(env.producer.gate)

	for _, results := range []<-chan startResult{replacement, other} {
		if result := <-results; result.err != nil {
			t.Errorf("held Start error = %v", result.err)
		}
	}
}

func TestStoppingTheOldSessionDuringItsReplacement(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	env.options.MaxTranscodeSessions = 1
	service := env.service(t)
	old := mustStart(t, service, request(transcodeFileID))
	results := startReplacementHeldAtTheGate(t, env, service, old.ID)

	if err := service.Stop(t.Context(), old.ID); err != nil {
		t.Fatalf("Stop of the old session error = %v", err)
	}

	// The replacement still holds its own slot after the old session is gone.
	if _, err := service.Start(t.Context(), request(audioTranscodeFileID)); !errors.Is(err, appplayback.ErrSessionLimitReached) {
		t.Errorf("third transcode error = %v, want ErrSessionLimitReached", err)
	}

	close(env.producer.gate)

	result := <-results
	if result.err != nil {
		t.Fatalf("replacement error = %v", result.err)
	}

	if stops := env.producer.productionOf(old.ID).stopCount(); stops != 1 {
		t.Errorf("old production stops = %d, want exactly 1", stops)
	}

	if _, err := service.Get(t.Context(), result.session.ID); err != nil {
		t.Errorf("Get of the replacement error = %v", err)
	}
}

func TestConcurrentStartAccessGetAndStop(t *testing.T) {
	t.Parallel()

	const rounds = 16

	env := newEnvironment()
	env.options.MaxTranscodeSessions = cappedFFmpegSessions
	service := env.service(t)
	fileIDs := []string{directFileID, remuxFileID, transcodeFileID, audioTranscodeFileID}

	var group sync.WaitGroup
	for round := range rounds {
		group.Go(func() {
			session, err := service.Start(t.Context(), request(fileIDs[round%len(fileIDs)]))
			if errors.Is(err, appplayback.ErrSessionLimitReached) {
				return
			}

			if err != nil {
				t.Errorf("Start error = %v", err)

				return
			}

			exerciseSession(t, service, session.ID)
		})
		group.Go(func() {
			// Seeks replacing sessions that may or may not exist race with everything else.
			seek := request(fileIDs[(round+1)%len(fileIDs)])
			seek.ReplacesSessionID = sessionID(round + 1)

			session, err := service.Start(t.Context(), seek)
			if errors.Is(err, appplayback.ErrSessionLimitReached) {
				return
			}

			if err != nil {
				t.Errorf("replacing Start error = %v", err)

				return
			}

			exerciseSession(t, service, session.ID)
		})
	}

	group.Wait()

	if created, removed := env.workspace.createdIDs(), env.workspace.removedIDs(); len(created) != len(removed) {
		t.Errorf("created %d workspaces, removed %d; want every one removed", len(created), len(removed))
	}
}

// exerciseSession reads, accesses and stops a session from several goroutines at once.
func exerciseSession(t *testing.T, service *appplayback.Service, id string) {
	t.Helper()

	var group sync.WaitGroup

	group.Go(func() {
		access, err := service.Access(t.Context(), id)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("Access error = %v", err)
		}

		access.Release()
	})
	group.Go(func() {
		if _, err := service.Get(t.Context(), id); err != nil && !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("Get error = %v", err)
		}
	})

	for range 2 {
		group.Go(func() {
			if err := service.Stop(t.Context(), id); err != nil {
				t.Errorf("Stop error = %v", err)
			}
		})
	}

	group.Wait()
}
