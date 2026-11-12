package playback_test

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

const throttleAhead = 60 * time.Second // ten segments

// throttled starts a transcode session of a service that throttles at throttleAhead.
func throttled(t *testing.T, env *environment) (*appplayback.Service, appplayback.Session, *fakeProduction) {
	t.Helper()

	env.options.ThrottleAhead = throttleAhead
	service := env.service(t)
	session := mustStart(t, service, request(transcodeFileID))

	synctest.Wait()

	return service, session, env.producer.productionOf(session.ID)
}

// produce sets the media time of the production to the given number of segments and runs one
// controller tick.
func produce(env *environment, production *fakeProduction, segments int) {
	production.setOutputTime(time.Duration(segments) * appplayback.SegmentDuration)
	env.clock.advance(appplayback.ThrottleInterval)
	synctest.Wait()
}

func listed(t *testing.T, service *appplayback.Service, id string) appplayback.SessionInfo {
	t.Helper()

	infos, err := service.List(adminContext(t))
	if err != nil {
		t.Fatalf("List error = %v", err)
	}

	for _, info := range infos {
		if info.ID == id {
			return info
		}
	}

	t.Fatalf("session %s missing from List", id)

	return appplayback.SessionInfo{}
}

func TestThrottlePausesAheadAndResumesWhenTheViewerCatchesUp(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		service, session, production := throttled(t, env)

		produce(env, production, 5)

		if production.isPaused() {
			t.Fatal("paused within the allowed lead")
		}

		produce(env, production, 20)

		if !production.isPaused() || !listed(t, service, session.ID).Paused {
			t.Fatal("not paused 20 segments ahead of a viewer at segment 0")
		}

		access, err := service.Access(adminContext(t), session.ID)
		if err != nil {
			t.Fatalf("Access error = %v", err)
		}

		defer access.Release()

		access.RequestSegment(16) // lead 4, below half of 10

		produce(env, production, 20)

		if production.isPaused() || listed(t, service, session.ID).Paused {
			t.Error("still paused after the viewer caught up")
		}
	})
}

func TestThrottleHysteresisDoesNotFlap(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		service, session, production := throttled(t, env)

		access, err := service.Access(adminContext(t), session.ID)
		if err != nil {
			t.Fatalf("Access error = %v", err)
		}

		defer access.Release()

		access.RequestSegment(10)
		produce(env, production, 20) // lead 10: not above the limit

		if production.isPaused() {
			t.Fatal("paused at exactly the allowed lead")
		}

		produce(env, production, 21) // lead 11 pauses

		access.RequestSegment(13) // lead 8: between half and the limit
		produce(env, production, 21)

		if !production.isPaused() {
			t.Error("resumed inside the hysteresis band")
		}

		access.RequestSegment(12) // never lowers the maximum
		produce(env, production, 21)

		if !production.isPaused() || production.pauses != 1 {
			t.Errorf("pauses = %d, paused = %v, want one pause", production.pauses, production.isPaused())
		}
	})
}

func TestThrottleSparesTheStartupBurst(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		env.options.ThrottleAhead = 20 * time.Second // four segments, burst of three is within
		service := env.service(t)
		session := mustStart(t, service, request(transcodeFileID))

		synctest.Wait()

		production := env.producer.productionOf(session.ID)
		produce(env, production, 3)

		if production.isPaused() {
			t.Error("paused during the startup burst")
		}
	})
}

func TestThrottleNeverPausesWhatCannotBePaused(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		env.producer.noPause = true
		_, _, production := throttled(t, env)

		produce(env, production, 50)

		if production.pauses != 0 {
			t.Errorf("pauses = %d, want 0 for an unsupported production", production.pauses)
		}
	})
}

func TestThrottleOffWithoutALead(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		service := env.service(t)
		session := mustStart(t, service, request(transcodeFileID))

		synctest.Wait()

		production := env.producer.productionOf(session.ID)
		produce(env, production, 50)

		if production.pauses != 0 {
			t.Errorf("pauses = %d, want 0 with throttling off", production.pauses)
		}
	})
}

func TestStopResumesAPausedProductionBeforeKilling(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		service, session, production := throttled(t, env)

		produce(env, production, 30)

		if !production.isPaused() {
			t.Fatal("production not paused")
		}

		if err := service.Stop(adminContext(t), session.ID); err != nil {
			t.Fatalf("Stop error = %v", err)
		}

		if production.pausedAtStop || production.resumes == 0 {
			t.Errorf("pausedAtStop = %v, resumes = %d, want resumed before the kill", production.pausedAtStop, production.resumes)
		}
	})
}

func TestIdleReaperReapsAPausedSession(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		service, session, production := throttled(t, env)

		produce(env, production, 30)
		env.passTime(appplayback.IdleTimeout + appplayback.IdleCheckInterval)

		if _, err := service.Get(adminContext(t), session.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("Get after idle timeout error = %v, want ErrNotFound", err)
		}

		if production.pausedAtStop || production.stopCount() == 0 {
			t.Errorf("pausedAtStop = %v, stops = %d, want resumed and stopped", production.pausedAtStop, production.stopCount())
		}
	})
}

func TestListShowsOperatorFieldsAndRespectsOwnership(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		service := env.service(t)
		owner, other := userContext(t, "auth-a", libraryID), userContext(t, "auth-b", libraryID)

		transcode, err := service.Start(owner, request(transcodeFileID))
		if err != nil {
			t.Fatalf("Start error = %v", err)
		}

		env.clock.advance(time.Second)

		if _, err := service.Start(other, request(directFileID)); err != nil {
			t.Fatalf("Start error = %v", err)
		}

		synctest.Wait()
		env.producer.productionOf(transcode.ID).setSpeedAndBitRate(1.5, 4_000_000)

		infos, err := service.List(owner)
		if err != nil || len(infos) != 1 {
			t.Fatalf("owner List = %v, %v, want exactly the own session", infos, err)
		}

		info := infos[0]
		if info.ID != transcode.ID || info.Mode != playback.ModeTranscode || info.Encoder == "" ||
			info.Speed != 1.5 || info.OutputBitRate != 4_000_000 || info.State != appplayback.StateReady ||
			info.ClientName != request(transcodeFileID).Profile.Name || !info.StartedAt.Equal(transcode.CreatedAt) {
			t.Errorf("info = %+v", info)
		}

		if all, err := service.List(adminContext(t)); err != nil || len(all) != 2 || all[0].ID != transcode.ID {
			t.Errorf("admin List = %v, %v, want both sessions, oldest first", all, err)
		}

		if _, err := service.List(t.Context()); !errors.Is(err, domain.ErrUnauthenticated) {
			t.Errorf("List without viewer error = %v, want ErrUnauthenticated", err)
		}
	})
}
