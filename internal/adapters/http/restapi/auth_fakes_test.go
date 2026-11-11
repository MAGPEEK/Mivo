package restapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/clientorigin"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	appaccess "github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/app/auth"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

const (
	authAccountID   = "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d"
	authPrimaryID   = "6f5e4d3c-2b1a-4f0e-9d8c-7b6a5f4e3d2c"
	authManagedID   = "7a6b5c4d-3e2f-4a1b-8c9d-0e1f2a3b4c5e"
	authSessionID   = "8e7d6c5b-4a3f-4e2d-9c1b-0a9f8e7d6c5b"
	authOtherDevice = "9f8e7d6c-5b4a-4f3e-8d2c-1b0a9f8e7d6c"

	authToken    = "token-of-the-new-session"
	authPassword = "correct horse battery"

	setupPath       = "/api/v1/setup"
	authSessionPath = "/api/v1/auth/session"
	mePath          = "/api/v1/me"
	meProfilePath   = "/api/v1/me/profile"
	mePasswordPath  = "/api/v1/me/password" //nolint:gosec // a route, no credential
	meDevicesPath   = "/api/v1/me/devices"

	firefoxOnWindows = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:132.0) Gecko/20100101 Firefox/132.0"
)

// fakeAuth is an in-memory restapi.AuthService: it answers with its fields and records what
// the handlers passed.
type fakeAuth struct {
	mutex sync.Mutex

	setupRequired bool
	setupErr      error
	signInErr     error
	meErr         error
	selectErr     error
	clearErr      error
	passwordErr   error
	devices       []auth.DeviceView
	devicesErr    error
	revokeErr     error
	// managed adds a managed profile, so no profile is selected at sign-in.
	managed bool

	signIns    []auth.SignInRequest
	setups     []auth.SignInRequest
	meViewers  []access.Viewer
	selections []auth.ProfileSelection
	changes    []auth.PasswordChange
	revoked    []string
	cleared    int
}

func (fake *fakeAuth) SetupRequired(context.Context) (bool, error) {
	return fake.setupRequired, fake.setupErr
}

func (fake *fakeAuth) Setup(_ context.Context, request auth.SignInRequest) (auth.SignInResult, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	fake.setups = append(fake.setups, request)

	return fake.result(request), fake.signInErr
}

func (fake *fakeAuth) SignIn(_ context.Context, request auth.SignInRequest) (auth.SignInResult, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	fake.signIns = append(fake.signIns, request)

	return fake.result(request), fake.signInErr
}

func (fake *fakeAuth) result(request auth.SignInRequest) auth.SignInResult {
	return auth.SignInResult{
		Token:           authToken,
		Session:         auth.Session{ID: authSessionID, AccountID: authAccountID, Device: request.Device},
		Account:         fakeAccount(),
		Profile:         fakePrimary(),
		ProfileSelected: !fake.managed,
	}
}

func (fake *fakeAuth) ChangePassword(_ context.Context, change auth.PasswordChange) error {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	fake.changes = append(fake.changes, change)

	return fake.passwordErr
}

func (fake *fakeAuth) Me(ctx context.Context) (auth.Me, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	viewer, _ := appaccess.ViewerFrom(ctx)
	fake.meViewers = append(fake.meViewers, viewer)

	if fake.meErr != nil {
		return auth.Me{}, fake.meErr
	}

	return fake.me(viewer.ProfileID), nil
}

func (fake *fakeAuth) me(profileID string) auth.Me {
	me := auth.Me{Account: fakeAccount(), Profiles: []auth.PickerProfile{{Profile: fakePrimary(), Admin: true}}}
	if fake.managed {
		me.Profiles = append(me.Profiles, auth.PickerProfile{Profile: fakeManaged()})
	}

	for _, entry := range me.Profiles {
		if entry.Profile.ID == profileID {
			me.Profile, me.ProfileSelected = entry.Profile, true
		}
	}

	return me
}

func (fake *fakeAuth) SelectProfile(_ context.Context, selection auth.ProfileSelection) (auth.Me, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	fake.selections = append(fake.selections, selection)

	if fake.selectErr != nil {
		return auth.Me{}, fake.selectErr
	}

	return fake.me(selection.ProfileID), nil
}

func (fake *fakeAuth) ClearProfile(context.Context) error {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	fake.cleared++

	return fake.clearErr
}

func (fake *fakeAuth) ListDevices(context.Context) ([]auth.DeviceView, error) {
	return fake.devices, fake.devicesErr
}

func (fake *fakeAuth) RevokeOwnDevice(_ context.Context, deviceID string) (bool, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	fake.revoked = append(fake.revoked, deviceID)

	return deviceID == authSessionID, fake.revokeErr
}

func fakeAccount() account.Account {
	return account.Account{ID: authAccountID, Name: "Anna", Role: account.RoleAdmin}
}

func fakePrimary() account.Profile {
	return account.Profile{ID: authPrimaryID, AccountID: authAccountID, Name: "Anna", Primary: true, AvatarColor: "coral"}
}

func fakeManaged() account.Profile {
	return account.Profile{ID: authManagedID, AccountID: authAccountID, Name: "Mia", AvatarColor: "teal", HasPIN: true}
}

func fakeDevices() []auth.DeviceView {
	seen := time.Date(2026, time.November, 9, 20, 15, 0, 0, time.UTC)

	return []auth.DeviceView{
		{
			ID: authSessionID, AccountID: authAccountID, LastSeenAt: seen, AddressClass: auth.AddressLAN, Current: true,
			Device: auth.Device{Name: "Firefox on Windows", Client: auth.ClientWeb, Platform: "Windows"},
		},
		{
			ID: authOtherDevice, AccountID: authAccountID, LastSeenAt: seen.Add(-time.Hour), AddressClass: auth.AddressRemote,
			Device: auth.Device{Name: "Living room TV", Client: auth.ClientTV},
		},
	}
}

// authRequest is one request to an account endpoint.
type authRequest struct {
	method    string
	target    string
	body      string
	userAgent string
	// tls marks the request as resolved to https (TLS socket or trusted proxy).
	tls bool
	// signedOut leaves the viewer out, as for the open sign-in and setup routes.
	signedOut bool
	peer      *auth.Peer
}

// serveAuth sends request as the session middleware would hand it on: with the viewer of
// the signed-in session unless signedOut, and with the peer when given.
func serveAuth(t *testing.T, service restapi.AuthService, request authRequest) (response, *syncBuffer) {
	t.Helper()

	handler, logs := newTestHandlerWithOptions(t, restapi.Options{Auth: service})

	var body *strings.Reader
	if request.body != "" {
		body = strings.NewReader(request.body)
	}

	ctx := t.Context()
	if !request.signedOut {
		ctx = appaccess.WithViewer(ctx, access.Viewer{AuthSessionID: authSessionID, AccountID: authAccountID, ProfileID: authPrimaryID})
	}

	if request.peer != nil {
		ctx = restapi.WithPeer(ctx, *request.peer)
	}

	// The HTTP server resolves the scheme; here the test plays its part.
	ctx = clientorigin.With(ctx, clientorigin.Origin{Secure: request.tls})

	var sent *http.Request
	if body == nil {
		sent = httptest.NewRequestWithContext(ctx, request.method, request.target, nil)
	} else {
		sent = httptest.NewRequestWithContext(ctx, request.method, request.target, body)
		sent.Header.Set("Content-Type", "application/json")
	}

	sent.Header.Set("User-Agent", request.userAgent)

	return serveRequest(t, handler, sent), logs
}

// sessionCookieOf returns the session cookie the response sets, if any.
func sessionCookieOf(t *testing.T, got response) *http.Cookie {
	t.Helper()

	recorded := http.Response{Header: got.header}
	for _, cookie := range recorded.Cookies() {
		if cookie.Name == restapi.SessionCookieName {
			return cookie
		}
	}

	return nil
}

func lanPeer() *auth.Peer {
	return &auth.Peer{Address: netip.MustParseAddr("192.168.1.20"), Class: auth.AddressLAN}
}
