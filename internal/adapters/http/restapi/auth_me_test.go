package restapi_test

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/app/auth"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/account"
)

func TestAuthGetMe(t *testing.T) {
	t.Parallel()

	got, _ := serveAuth(t, &fakeAuth{managed: true}, authRequest{method: http.MethodGet, target: mePath})
	assertJSONResponse(t, got, http.StatusOK)

	me := decodeStrictly[apigen.Me](t, got.body)
	if me.Account.ID.String() != authAccountID || me.Account.Name != "Anna" || me.Account.Role != "admin" {
		t.Errorf("account = %+v", me.Account)
	}

	if me.CurrentProfile == nil || me.CurrentProfile.ID.String() != authPrimaryID || !me.CurrentProfile.IsAdmin || !me.CurrentProfile.IsPrimary {
		t.Errorf("current profile = %+v, want the admin's primary profile", me.CurrentProfile)
	}

	want := apigen.ProfileSummary{Name: "Mia", AvatarColor: "teal", HasPin: true}
	if len(me.Profiles) != 2 || me.Profiles[1].ID.String() != authManagedID {
		t.Fatalf("profiles = %+v, want the primary and the managed profile", me.Profiles)
	}

	want.ID = me.Profiles[1].ID
	if me.Profiles[1] != want {
		t.Errorf("managed profile = %+v, want %+v", me.Profiles[1], want)
	}
}

func TestAuthGetMeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "session gone", err: fmt.Errorf("me: %w", domain.ErrUnauthenticated), wantStatus: 401, wantCode: restapi.CodeUnauthenticated},
		{name: "store failure", err: errors.New("database locked"), wantStatus: 500, wantCode: restapi.CodeInternal},
		{name: "unpublished sentinel", err: domain.ErrNotFound, wantStatus: 500, wantCode: restapi.CodeInternal},
	}

	for _, test := range tests {
		got, _ := serveAuth(t, &fakeAuth{meErr: test.err}, authRequest{method: http.MethodGet, target: mePath})
		assertProblem(t, got, test.wantStatus, test.wantCode)
	}
}

func TestAuthSelectProfile(t *testing.T) {
	t.Parallel()

	service := &fakeAuth{managed: true}
	got, _ := serveAuth(t, service, authRequest{
		method: http.MethodPut, target: meProfilePath,
		body: `{"profileId":"` + authManagedID + `","pin":"1234","remember":false}`,
	})
	assertJSONResponse(t, got, http.StatusOK)

	me := decodeStrictly[apigen.Me](t, got.body)
	if me.CurrentProfile == nil || me.CurrentProfile.ID.String() != authManagedID || me.CurrentProfile.IsAdmin {
		t.Errorf("current profile = %+v, want the managed profile without admin powers", me.CurrentProfile)
	}

	if len(service.selections) != 1 {
		t.Fatalf("selections = %+v, want one", service.selections)
	}

	selection := service.selections[0]
	if selection.ProfileID != authManagedID || selection.PIN != "1234" || selection.Remember == nil || *selection.Remember {
		t.Errorf("selection = %+v, want the profile, the PIN and remember false", selection)
	}

	// An omitted remember stays undecided, so the use case applies its default.
	_, _ = serveAuth(t, service, authRequest{method: http.MethodPut, target: meProfilePath, body: `{"profileId":"` + authPrimaryID + `"}`})

	if last := service.selections[len(service.selections)-1]; last.Remember != nil || last.PIN != "" {
		t.Errorf("selection = %+v, want no PIN and an undecided remember", last)
	}
}

func TestAuthSelectProfileErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantRetry  string
	}{
		{name: "wrong pin", err: fmt.Errorf("select profile: %w", auth.ErrPINInvalid), wantStatus: 403, wantCode: restapi.CodePINInvalid},
		{
			name: "locked", err: fmt.Errorf("select profile: %w", &auth.ProfileLockedError{Wait: 5 * time.Minute}),
			wantStatus: 429, wantCode: restapi.CodeProfileLocked, wantRetry: "300",
		},
		{name: "foreign profile", err: fmt.Errorf("select profile: %w", domain.ErrNotFound), wantStatus: 404, wantCode: restapi.CodeNotFound},
		{name: "session gone", err: domain.ErrUnauthenticated, wantStatus: 401, wantCode: restapi.CodeUnauthenticated},
		{name: "store failure", err: errors.New("database locked"), wantStatus: 500, wantCode: restapi.CodeInternal},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, _ := serveAuth(t, &fakeAuth{selectErr: test.err}, authRequest{
				method: http.MethodPut, target: meProfilePath, body: `{"profileId":"` + authManagedID + `","pin":"0000"}`,
			})

			problem := assertProblem(t, got, test.wantStatus, test.wantCode)
			if got.header.Get("Retry-After") != test.wantRetry || (test.wantRetry != "") != (problem.RetryAfterSeconds != nil) {
				t.Errorf("Retry-After = %q, retryAfterSeconds = %v, want %q", got.header.Get("Retry-After"), problem.RetryAfterSeconds, test.wantRetry)
			}
		})
	}
}

func TestAuthClearProfileSelection(t *testing.T) {
	t.Parallel()

	service := &fakeAuth{}
	got, _ := serveAuth(t, service, authRequest{method: http.MethodDelete, target: meProfilePath})

	if got.status != http.StatusNoContent || service.cleared != 1 {
		t.Errorf("answer = %d after %d clears, want 204 after one", got.status, service.cleared)
	}

	// The session stays signed in, so the cookie is left alone.
	if got.header.Get("Set-Cookie") != "" {
		t.Errorf("Set-Cookie = %q, want none", got.header.Get("Set-Cookie"))
	}

	got, _ = serveAuth(t, &fakeAuth{clearErr: domain.ErrUnauthenticated}, authRequest{method: http.MethodDelete, target: meProfilePath})
	assertProblem(t, got, http.StatusUnauthorized, restapi.CodeUnauthenticated)
}

func TestAuthChangeOwnPassword(t *testing.T) {
	t.Parallel()

	service := &fakeAuth{}
	got, _ := serveAuth(t, service, authRequest{
		method: http.MethodPut, target: mePasswordPath, peer: lanPeer(),
		body: `{"currentPassword":"` + authPassword + `","newPassword":"another long passphrase"}`,
	})

	if got.status != http.StatusNoContent || len(got.body) != 0 {
		t.Fatalf("answer = %d %s, want 204", got.status, got.body)
	}

	want := auth.PasswordChange{Current: authPassword, New: "another long passphrase", Peer: *lanPeer()}
	if len(service.changes) != 1 || service.changes[0] != want {
		t.Errorf("change = %+v, want %+v", service.changes, want)
	}
}

func TestAuthChangeOwnPasswordErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantField  string
		wantRetry  string
	}{
		// 403, not 401: the session stays signed in.
		{name: "wrong current password", err: fmt.Errorf("change password: %w", auth.ErrInvalidCredentials), wantStatus: 403, wantCode: restapi.CodeInvalidCredentials},
		{
			name: "invalid new password", err: fmt.Errorf("change password: %w", &account.InvalidFieldError{Field: account.FieldPassword, Reason: "too short"}),
			wantStatus: 400, wantCode: restapi.CodeInvalid, wantField: "newPassword",
		},
		{name: "managed profile", err: fmt.Errorf("change password: %w", domain.ErrForbidden), wantStatus: 403, wantCode: restapi.CodeForbidden},
		{name: "throttled", err: &auth.ThrottledError{Wait: 30 * time.Second}, wantStatus: 429, wantCode: restapi.CodeTooManyAttempts, wantRetry: "30"},
		{name: "session gone", err: domain.ErrUnauthenticated, wantStatus: 401, wantCode: restapi.CodeUnauthenticated},
		{name: "store failure", err: errors.New("database locked"), wantStatus: 500, wantCode: restapi.CodeInternal},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, _ := serveAuth(t, &fakeAuth{passwordErr: test.err}, authRequest{
				method: http.MethodPut, target: mePasswordPath, body: `{"currentPassword":"a","newPassword":"b"}`,
			})

			problem := assertProblem(t, got, test.wantStatus, test.wantCode)
			assertProblemParts(t, problem, test.wantField, false)

			if got.header.Get("Retry-After") != test.wantRetry {
				t.Errorf("Retry-After = %q, want %q", got.header.Get("Retry-After"), test.wantRetry)
			}
		})
	}
}

// TestAuthListOwnDevicesExposesNoSessionSecrets: the list has exactly the contract's fields
// (BACKLOG m7-09: never the token hash).
func TestAuthListOwnDevicesExposesNoSessionSecrets(t *testing.T) {
	t.Parallel()

	got, _ := serveAuth(t, &fakeAuth{devices: fakeDevices()}, authRequest{method: http.MethodGet, target: meDevicesPath})
	assertJSONResponse(t, got, http.StatusOK)

	list := decodeStrictly[apigen.DeviceList](t, got.body)
	if len(list.Devices) != 2 {
		t.Fatalf("devices = %+v, want two", list.Devices)
	}

	current, other := list.Devices[0], list.Devices[1]
	if current.ID.String() != authSessionID || !current.Current || current.Name != "Firefox on Windows" || current.Client != "web" ||
		current.Platform == nil || *current.Platform != "Windows" || current.AddressClass != "lan" ||
		!current.LastSeenAt.Equal(fakeDevices()[0].LastSeenAt) {
		t.Errorf("current device = %+v", current)
	}

	if other.ID.String() != authOtherDevice || other.Current || other.Client != "tv" || other.Platform != nil || other.AddressClass != "remote" {
		t.Errorf("other device = %+v", other)
	}

	for _, forbidden := range []string{"token", "hash", "accountId"} {
		if strings.Contains(strings.ToLower(string(got.body)), strings.ToLower(forbidden)) {
			t.Errorf("device list %s mentions %q", got.body, forbidden)
		}
	}
}

func TestAuthListOwnDevicesEdges(t *testing.T) {
	t.Parallel()

	got, _ := serveAuth(t, &fakeAuth{}, authRequest{method: http.MethodGet, target: meDevicesPath})
	if strings.TrimSpace(string(got.body)) != `{"devices":[]}` {
		t.Errorf("empty list = %s, want an empty array", got.body)
	}

	got, _ = serveAuth(t, &fakeAuth{devicesErr: domain.ErrUnauthenticated}, authRequest{method: http.MethodGet, target: meDevicesPath})
	assertProblem(t, got, http.StatusUnauthorized, restapi.CodeUnauthenticated)

	broken := []auth.DeviceView{{ID: "no uuid"}}
	got, _ = serveAuth(t, &fakeAuth{devices: broken}, authRequest{method: http.MethodGet, target: meDevicesPath})
	assertProblem(t, got, http.StatusInternalServerError, restapi.CodeInternal)
}

func TestAuthRevokeOwnDevice(t *testing.T) {
	t.Parallel()

	service := &fakeAuth{}
	got, _ := serveAuth(t, service, authRequest{method: http.MethodDelete, target: meDevicesPath + "/" + authOtherDevice})

	if got.status != http.StatusNoContent || got.header.Get("Set-Cookie") != "" {
		t.Errorf("answer = %d cookie %q, want 204 and the caller's cookie kept", got.status, got.header.Get("Set-Cookie"))
	}

	if len(service.revoked) != 1 || service.revoked[0] != authOtherDevice {
		t.Errorf("revoked = %v, want the other device", service.revoked)
	}

	// Revoking the current device equals signing out.
	got, _ = serveAuth(t, &fakeAuth{}, authRequest{method: http.MethodDelete, target: meDevicesPath + "/" + authSessionID})
	if cookie := sessionCookieOf(t, got); got.status != http.StatusNoContent || cookie == nil || cookie.MaxAge >= 0 || cookie.Value != "" {
		t.Errorf("answer = %d cookie %+v, want 204 and the expired cookie", got.status, cookie)
	}
}

func TestAuthRevokeOwnDeviceErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		target     string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "foreign or unknown device", err: fmt.Errorf("revoke device: %w", domain.ErrNotFound), wantStatus: 404, wantCode: restapi.CodeNotFound},
		{name: "session gone", err: domain.ErrUnauthenticated, wantStatus: 401, wantCode: restapi.CodeUnauthenticated},
		{name: "store failure", err: errors.New("database locked"), wantStatus: 500, wantCode: restapi.CodeInternal},
		{name: "malformed id", target: meDevicesPath + "/not-a-uuid", wantStatus: 400, wantCode: restapi.CodeBadRequest},
	}

	for _, test := range tests {
		target := test.target
		if target == "" {
			target = meDevicesPath + "/" + authOtherDevice
		}

		got, _ := serveAuth(t, &fakeAuth{revokeErr: test.err}, authRequest{method: http.MethodDelete, target: target})
		assertProblem(t, got, test.wantStatus, test.wantCode)
	}
}
