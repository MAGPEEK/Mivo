package playback_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
)

func TestStartWithoutViewerFailsClosed(t *testing.T) {
	t.Parallel()

	env := newEnvironment()

	_, err := env.service(t).Start(t.Context(), request(directFileID))
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("Start error = %v, want ErrUnauthenticated", err)
	}

	if env.files.loadCount() != 0 {
		t.Error("the catalog was read without a viewer")
	}
}

func TestStartRefusesAFileTheViewerMayNotSee(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	env.files.hidden = map[string]bool{directFileID: true}
	service := env.service(t)

	// The file of a hidden item (itself or through an ancestor) answers as an unknown file.
	_, err := service.Start(userContext(t, "auth-a", libraryID), request(directFileID))
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("hidden file: error = %v, want ErrNotFound", err)
	}

	// A library outside the viewer's access set answers the same.
	_, err = service.Start(userContext(t, "auth-a", "another-library"), request(remuxFileID))
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("file of another library: error = %v, want ErrNotFound", err)
	}

	// Nothing was started: no slot, no session.
	if started := len(env.producer.jobs()); started != 0 {
		t.Errorf("productions started = %d, want 0", started)
	}

	// An allowed file still starts, and the catalog saw the viewer's restriction.
	if _, err := service.Start(userContext(t, "auth-a", libraryID), request(remuxFileID)); err != nil {
		t.Fatalf("allowed file: %v", err)
	}

	last := env.files.restrictions[len(env.files.restrictions)-1]
	if last.AllLibraries() || !last.AllowsLibrary(libraryID) {
		t.Errorf("restriction = %+v, want the viewer's library set", last)
	}
}

func TestASessionBelongsToTheAuthSessionThatStartedIt(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	service := env.service(t)
	owner := userContext(t, "auth-a", libraryID)
	other := userContext(t, "auth-b", libraryID)

	started, err := service.Start(owner, request(remuxFileID))
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}

	if _, err := service.Get(other, started.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get by another auth session: %v, want ErrNotFound", err)
	}

	if _, err := service.Access(other, started.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Access by another auth session: %v, want ErrNotFound", err)
	}

	if err := service.Stop(other, started.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Stop by another auth session: %v, want ErrNotFound", err)
	}

	// The attempts changed nothing: the owner and an administrator still reach the session.
	if _, err := service.Get(owner, started.ID); err != nil {
		t.Errorf("Get by the owner: %v", err)
	}

	if _, err := service.Get(adminContext(t), started.ID); err != nil {
		t.Errorf("Get by an administrator: %v", err)
	}

	access, err := service.Access(owner, started.ID)
	if err != nil {
		t.Fatalf("Access by the owner: %v", err)
	}

	access.Release()

	for name, call := range map[string]func() error{
		"Get":    func() error { _, err := service.Get(t.Context(), started.ID); return err },
		"Access": func() error { _, err := service.Access(t.Context(), started.ID); return err },
		"Stop":   func() error { return service.Stop(t.Context(), started.ID) },
	} {
		if err := call(); !errors.Is(err, domain.ErrUnauthenticated) {
			t.Errorf("%s without a viewer: %v, want ErrUnauthenticated", name, err)
		}
	}

	if err := service.Stop(adminContext(t), started.ID); err != nil {
		t.Errorf("Stop by an administrator: %v", err)
	}

	if _, err := service.Get(owner, started.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get after the stop: %v, want ErrNotFound", err)
	}
}

func TestASeekCannotReplaceTheSessionOfAnotherAuthSession(t *testing.T) {
	t.Parallel()

	env := newEnvironment()
	service := env.service(t)

	victim, err := service.Start(userContext(t, "auth-a", libraryID), request(remuxFileID))
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}

	seek := request(remuxFileID)
	seek.ReplacesSessionID = victim.ID

	if _, err := service.Start(userContext(t, "auth-b", libraryID), seek); err != nil {
		t.Fatalf("Start with a foreign replace id error = %v", err)
	}

	if _, err := service.Get(userContext(t, "auth-a", libraryID), victim.ID); err != nil {
		t.Errorf("the other viewer's session was replaced: %v", err)
	}
}
