// Package activity answers what the server does right now: the running library scans and the
// playback sessions, for the activity indicator and the administrator's "Now playing" view
// (M5-16). It owns no state; it joins what the scan service and the session registry
// already keep.
package activity

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	appaccess "github.com/magpeek/mivo/internal/app/access"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
)

// ScanActivity lists the scans that run now.
type ScanActivity interface {
	Running() []scan.Running
}

// SessionActivity lists the sessions the viewer of ctx may see.
type SessionActivity interface {
	List(ctx context.Context) ([]appplayback.SessionInfo, error)
}

// Item is the catalog item a media file belongs to.
type Item struct {
	ID    string
	Title string
}

// ItemTitles names the item of a media file without a visibility restriction; the caller is
// an administrator, who sees the whole catalog. An unknown file wraps domain.ErrNotFound.
type ItemTitles interface {
	ItemOfFile(ctx context.Context, fileID string) (Item, error)
}

// Dependencies are the ports of a Service; all are required.
type Dependencies struct {
	Scans    ScanActivity
	Sessions SessionActivity
	Items    ItemTitles
	// Logger records a title lookup that failed; nil discards. The snapshot then shows the
	// session without a title instead of failing as a whole.
	Logger *slog.Logger
}

// Session is one running playback session with the title of its item. ItemID and Title are
// empty when the item is gone from the catalog; the session stays listed so that an
// administrator can still see and stop it.
type Session struct {
	Info   appplayback.SessionInfo
	ItemID string
	Title  string
}

// Snapshot is the activity at one moment. Both lists are never nil.
type Snapshot struct {
	Scans    []scan.Running
	Sessions []Session
}

// Service builds the activity snapshot.
type Service struct {
	scans    ScanActivity
	sessions SessionActivity
	items    ItemTitles
	logger   *slog.Logger
}

// NewService returns a Service. A missing port is a wiring bug and wraps domain.ErrInvalid.
func NewService(dependencies Dependencies) (*Service, error) {
	if dependencies.Scans == nil || dependencies.Sessions == nil || dependencies.Items == nil {
		return nil, fmt.Errorf("activity service needs scans, sessions and items: %w", domain.ErrInvalid)
	}

	logger := dependencies.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	return &Service{scans: dependencies.Scans, sessions: dependencies.Sessions, items: dependencies.Items, logger: logger}, nil
}

// Snapshot returns the running scans and sessions. Only an administrator may ask: the view
// names every viewer's titles, so it fails closed in the use case as well as in the HTTP
// guard (ADR 0032 section 9). Without a viewer the error wraps domain.ErrUnauthenticated, for
// anyone else domain.ErrForbidden.
func (s *Service) Snapshot(ctx context.Context) (Snapshot, error) {
	viewer, ok := appaccess.ViewerFrom(ctx)
	if !ok {
		return Snapshot{}, fmt.Errorf("viewer: %w", domain.ErrUnauthenticated)
	}

	if !viewer.Admin {
		return Snapshot{}, fmt.Errorf("activity needs an administrator: %w", domain.ErrForbidden)
	}

	infos, err := s.sessions.List(ctx)
	if err != nil {
		return Snapshot{}, fmt.Errorf("list sessions: %w", err)
	}

	sessions := make([]Session, 0, len(infos))

	for _, info := range infos {
		sessions = append(sessions, s.session(ctx, info))
	}

	scans := s.scans.Running()
	if scans == nil {
		scans = []scan.Running{}
	}

	return Snapshot{Scans: scans, Sessions: sessions}, nil
}

// session joins one session with its item. A missing item and a failed lookup both leave the
// title empty; only the failure is logged, so one broken lookup never hides the others.
func (s *Service) session(ctx context.Context, info appplayback.SessionInfo) Session {
	item, err := s.items.ItemOfFile(ctx, info.FileID)

	switch {
	case err == nil:
		return Session{Info: info, ItemID: item.ID, Title: item.Title}
	case errors.Is(err, domain.ErrNotFound):
		return Session{Info: info}
	default:
		s.logger.WarnContext(ctx, "activity: item lookup failed, session shown without title",
			slog.String("session", info.ID), slog.String("error", err.Error()))

		return Session{Info: info}
	}
}
