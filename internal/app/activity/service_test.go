package activity_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	appaccess "github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/app/activity"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/playback"
)

type fakeScans struct{ running []scan.Running }

func (fake fakeScans) Running() []scan.Running { return fake.running }

type fakeSessions struct {
	infos []appplayback.SessionInfo
	err   error
	calls int
}

func (fake *fakeSessions) List(context.Context) ([]appplayback.SessionInfo, error) {
	fake.calls++

	return fake.infos, fake.err
}

type fakeItems struct {
	items map[string]activity.Item
	err   error
}

func (fake fakeItems) ItemOfFile(_ context.Context, fileID string) (activity.Item, error) {
	if fake.err != nil {
		return activity.Item{}, fake.err
	}

	item, found := fake.items[fileID]
	if !found {
		return activity.Item{}, fmt.Errorf("file %s: %w", fileID, domain.ErrNotFound)
	}

	return item, nil
}

func adminContext(t *testing.T) context.Context {
	t.Helper()

	return appaccess.WithViewer(t.Context(), access.Viewer{AuthSessionID: "auth-a", Admin: true})
}

func newService(t *testing.T, scans fakeScans, sessions *fakeSessions, items fakeItems) *activity.Service {
	t.Helper()

	service, err := activity.NewService(activity.Dependencies{Scans: scans, Sessions: sessions, Items: items})
	if err != nil {
		t.Fatalf("NewService error = %v", err)
	}

	return service
}

func TestSnapshotOfAnIdleServerHasEmptyLists(t *testing.T) {
	t.Parallel()

	got, err := newService(t, fakeScans{}, &fakeSessions{}, fakeItems{}).Snapshot(adminContext(t))
	if err != nil {
		t.Fatalf("Snapshot error = %v", err)
	}

	if got.Scans == nil || got.Sessions == nil || len(got.Scans) != 0 || len(got.Sessions) != 0 {
		t.Errorf("Snapshot = %+v, want empty non-nil lists", got)
	}
}

func TestSnapshotJoinsScansSessionsAndTitles(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	scans := fakeScans{running: []scan.Running{{
		LibraryID: "lib-1", LibraryName: "Movies",
		Status: scan.Status{State: scan.StateRunning, StartedAt: started, Counts: scan.Counts{Added: 3, Unchanged: 40}},
	}}}
	sessions := &fakeSessions{infos: []appplayback.SessionInfo{
		{
			ID: "s-1", FileID: "file-1", State: appplayback.StateReady, Mode: playback.ModeTranscode,
			Reasons:    []playback.Reason{{Code: playback.ReasonVideoCodecNotSupported, Scope: playback.ScopeVideo}},
			Encoder:    "libx264",
			CPUPercent: 120, ClientName: "chromium", Local: false, StartedAt: started,
		},
		{ID: "s-2", FileID: "file-gone", State: appplayback.StateReady, Mode: playback.ModeDirectPlay, StartedAt: started},
	}}
	items := fakeItems{items: map[string]activity.Item{"file-1": {ID: "item-1", Title: "Big Buck Bunny"}}}

	got, err := newService(t, scans, sessions, items).Snapshot(adminContext(t))
	if err != nil {
		t.Fatalf("Snapshot error = %v", err)
	}

	if len(got.Scans) != 1 || got.Scans[0].LibraryName != "Movies" || got.Scans[0].Status.Counts.Added != 3 {
		t.Errorf("scans = %+v", got.Scans)
	}

	if len(got.Sessions) != 2 {
		t.Fatalf("sessions = %+v, want 2", got.Sessions)
	}

	first := got.Sessions[0]
	if first.ItemID != "item-1" || first.Title != "Big Buck Bunny" || first.Info.CPUPercent != 120 || first.Info.Local ||
		len(first.Info.Reasons) != 1 {
		t.Errorf("first session = %+v", first)
	}

	if gone := got.Sessions[1]; gone.ItemID != "" || gone.Title != "" {
		t.Errorf("session of a deleted item = %+v, want no item and no title", gone)
	}
}

func TestSnapshotRefusesEveryoneButAdministrators(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		context func(*testing.T) context.Context
		want    error
	}{
		{"no viewer", func(t *testing.T) context.Context { return t.Context() }, domain.ErrUnauthenticated},
		{"managed profile", func(t *testing.T) context.Context {
			return appaccess.WithViewer(t.Context(), access.Viewer{AuthSessionID: "auth-b"})
		}, domain.ErrForbidden},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			sessions := &fakeSessions{infos: []appplayback.SessionInfo{{ID: "s-1", FileID: "f"}}}
			items := fakeItems{items: map[string]activity.Item{"f": {ID: "i", Title: "Hidden Title"}}}

			got, err := newService(t, fakeScans{running: []scan.Running{{LibraryID: "l"}}}, sessions, items).Snapshot(test.context(t))
			if !errors.Is(err, test.want) {
				t.Fatalf("Snapshot error = %v, want %v", err, test.want)
			}

			if len(got.Scans) != 0 || len(got.Sessions) != 0 || sessions.calls != 0 {
				t.Errorf("Snapshot leaked data before the check: %+v, session calls %d", got, sessions.calls)
			}
		})
	}
}

func TestSnapshotReportsPortFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")

	if _, err := newService(t, fakeScans{}, &fakeSessions{err: boom}, fakeItems{}).Snapshot(adminContext(t)); !errors.Is(err, boom) {
		t.Errorf("session failure error = %v, want boom", err)
	}
}

func TestSnapshotShowsASessionWithoutTitleWhenTheItemLookupFails(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer

	sessions := &fakeSessions{infos: []appplayback.SessionInfo{{ID: "s", FileID: "f"}}}
	service, err := activity.NewService(activity.Dependencies{
		Scans: fakeScans{}, Sessions: sessions, Items: fakeItems{err: errors.New("database is locked")},
		Logger: slog.New(slog.NewTextHandler(&logs, nil)),
	})
	if err != nil {
		t.Fatalf("NewService error = %v", err)
	}

	got, err := service.Snapshot(adminContext(t))
	if err != nil || len(got.Sessions) != 1 || got.Sessions[0].Title != "" || got.Sessions[0].Info.ID != "s" {
		t.Fatalf("Snapshot = %+v, %v, want the session without a title", got, err)
	}

	if !strings.Contains(logs.String(), "database is locked") {
		t.Errorf("log %q does not record the failed lookup", logs.String())
	}
}

func TestNewServiceRejectsMissingPorts(t *testing.T) {
	t.Parallel()

	full := activity.Dependencies{Scans: fakeScans{}, Sessions: &fakeSessions{}, Items: fakeItems{}}

	tests := map[string]func(*activity.Dependencies){
		"scans":    func(d *activity.Dependencies) { d.Scans = nil },
		"sessions": func(d *activity.Dependencies) { d.Sessions = nil },
		"items":    func(d *activity.Dependencies) { d.Items = nil },
	}

	for name, mutate := range tests {
		dependencies := full
		mutate(&dependencies)

		if _, err := activity.NewService(dependencies); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: NewService error = %v, want ErrInvalid", name, err)
		}
	}
}
