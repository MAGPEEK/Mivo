package restapi_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/app/activity"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

var _ restapi.ActivityService = (*activity.Service)(nil)

const (
	activityPath      = "/api/v1/activity"
	activityLibraryID = "3f2b8c1e-7d4a-4c59-9a1e-2b6f0d8e4a71"
)

type fakeActivity struct {
	snapshot activity.Snapshot
	err      error
}

func (fake fakeActivity) Snapshot(context.Context) (activity.Snapshot, error) {
	return fake.snapshot, fake.err
}

func activityHandler(t *testing.T, service restapi.ActivityService) http.Handler {
	t.Helper()

	handler, _ := newTestHandlerWithOptions(t, restapi.Options{Activity: service})

	return handler
}

func TestActivityMapsScansAndSessions(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	snapshot := activity.Snapshot{
		Scans: []scan.Running{{
			LibraryID: activityLibraryID, LibraryName: "Movies",
			Status: scan.Status{State: scan.StateRunning, StartedAt: started, Counts: scan.Counts{Added: 3, Unchanged: 40}},
		}},
		Sessions: []activity.Session{
			{
				ItemID: testItemID, Title: testItemTitle,
				Info: appplayback.SessionInfo{
					ID: testSessionID, FileID: testFileID, State: appplayback.StateReady, Mode: playback.ModeTranscode,
					Reasons: []playback.Reason{{Code: playback.ReasonVideoCodecNotSupported, Scope: playback.ScopeVideo}},
					Encoder: "libx264", CPUPercent: 183.5, ClientName: "chromium", Local: false, StartedAt: started,
				},
			},
			{Info: appplayback.SessionInfo{
				ID: "6e5d4c3b-2a1f-4e0d-9c8b-7a6f5e4d3c2b", FileID: testFileID, State: appplayback.StateReady,
				Mode: playback.ModeDirectPlay, ClientName: "tv", Local: true, StartedAt: started,
			}},
		},
	}

	got := serve(t, activityHandler(t, fakeActivity{snapshot: snapshot}), http.MethodGet, activityPath)
	if got.status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", got.status, got.body)
	}

	body := decode[apigen.ActivityStatus](t, got.body)
	if len(body.Scans) != 1 || body.Scans[0].LibraryID.String() != activityLibraryID || body.Scans[0].LibraryName != "Movies" ||
		body.Scans[0].Counts.Added != 3 || !body.Scans[0].StartedAt.Equal(started) {
		t.Errorf("scans = %+v", body.Scans)
	}

	if len(body.Sessions) != 2 {
		t.Fatalf("sessions = %+v, want 2", body.Sessions)
	}

	first := body.Sessions[0]
	if first.Title != testItemTitle || first.ItemID.String() != testItemID || first.ClientClass != apigen.ActivityClientClassRemote ||
		len(first.Reasons) != 1 || string(first.Reasons[0].Code) != string(playback.ReasonVideoCodecNotSupported) ||
		first.CPUPercent == nil || *first.CPUPercent != 183.5 || first.VideoEncoder == nil || *first.VideoEncoder != "libx264" {
		t.Errorf("first session = %+v", first)
	}

	second := body.Sessions[1]
	if second.Title != "" || second.ClientClass != apigen.ActivityClientClassLan || second.CPUPercent != nil || len(second.Reasons) != 0 {
		t.Errorf("second session = %+v, want no title, lan, no CPU, no reasons", second)
	}
}

func TestActivityIsEmptyListsAndNeverCarriesPaths(t *testing.T) {
	t.Parallel()

	got := serve(t, activityHandler(t, fakeActivity{}), http.MethodGet, activityPath)
	if got.status != http.StatusOK || strings.TrimSpace(string(got.body)) != `{"scans":[],"sessions":[]}` {
		t.Errorf("idle = %d %s", got.status, got.body)
	}

	snapshot := activity.Snapshot{Sessions: []activity.Session{{
		ItemID: testItemID, Title: testItemTitle,
		Info: appplayback.SessionInfo{ID: testSessionID, FileID: testFileID, Mode: playback.ModeRemux, ClientName: "tv"},
	}}}

	body := string(serve(t, activityHandler(t, fakeActivity{snapshot: snapshot}), http.MethodGet, activityPath).body)
	for _, forbidden := range []string{"streamUrl", "fileName", "fileId", "/media", "path", testFileID} {
		if strings.Contains(body, forbidden) {
			t.Errorf("body %s contains %q", body, forbidden)
		}
	}
}

func TestActivityErrors(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		service    fakeActivity
		wantStatus int
		wantCode   string
	}{
		"unauthenticated": {fakeActivity{err: fmt.Errorf("viewer: %w", domain.ErrUnauthenticated)}, http.StatusUnauthorized, "unauthenticated"},
		"not an admin":    {fakeActivity{err: fmt.Errorf("admin: %w", domain.ErrForbidden)}, http.StatusForbidden, "forbidden"},
		"failure":         {fakeActivity{err: errors.New("boom /srv/media")}, http.StatusInternalServerError, "internal_error"},
		"bad session id": {fakeActivity{snapshot: activity.Snapshot{Sessions: []activity.Session{{
			Info: appplayback.SessionInfo{ID: "nope", Mode: playback.ModeRemux},
		}}}}, http.StatusInternalServerError, "internal_error"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := serve(t, activityHandler(t, test.service), http.MethodGet, activityPath)
			assertProblem(t, got, test.wantStatus, test.wantCode)

			if strings.Contains(string(got.body), "/srv/media") {
				t.Errorf("body leaks the error text: %s", got.body)
			}
		})
	}
}

func TestActivityAnswersNotImplementedWithoutService(t *testing.T) {
	t.Parallel()

	handler, _ := newTestHandlerWithOptions(t, restapi.Options{})

	assertProblem(t, serve(t, handler, http.MethodGet, activityPath), http.StatusNotImplemented, "not_implemented")
}
