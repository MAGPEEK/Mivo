package playback

import (
	"context"
	"fmt"

	appaccess "github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
)

// ownerOf returns the auth session that started the visible session; false when there is no
// such session.
func (sessions *registry) ownerOf(id string) (string, bool) {
	sessions.mutex.Lock()
	defer sessions.mutex.Unlock()

	entry, found := sessions.visible(id)
	if !found {
		return "", false
	}

	return entry.owner, true
}

// mayReach reports whether the viewer may reach a session started by owner: an administrator
// reaches every session, anyone else only the sessions of their own sign-in session. A
// viewer without a sign-in session id reaches none (fail closed).
func mayReach(viewer access.Viewer, owner string) bool {
	return viewer.Admin || (viewer.AuthSessionID != "" && viewer.AuthSessionID == owner)
}

// authorize checks that the viewer of ctx may reach the session. Without a viewer it
// reports domain.ErrUnauthenticated; a session of another sign-in session answers like a
// missing one, domain.ErrNotFound, so its ID reveals nothing. An unknown session passes: the
// operation reports it (or, for Stop, treats it as already stopped). The owner never changes,
// so the check cannot race with the operation that follows.
func (s *Service) authorize(ctx context.Context, id string) error {
	viewer, ok := appaccess.ViewerFrom(ctx)
	if !ok {
		return fmt.Errorf("viewer: %w", domain.ErrUnauthenticated)
	}

	owner, found := s.sessions.ownerOf(id)
	if found && !mayReach(viewer, owner) {
		return fmt.Errorf("playback session %s: %w", id, domain.ErrNotFound)
	}

	return nil
}

// replaceableBy returns the session id a new session of the viewer may replace: a session
// of another sign-in session is left alone and the id is dropped, exactly as for an unknown
// session, so a seek cannot end somebody else's playback.
func (s *Service) replaceableBy(viewer access.Viewer, id string) string {
	if id == "" {
		return ""
	}

	if owner, found := s.sessions.ownerOf(id); found && !mayReach(viewer, owner) {
		return ""
	}

	return id
}
