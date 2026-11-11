package restapi

import (
	"context"
	"net/http"
	"slices"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/clientorigin"
	"github.com/magpeek/mivo/internal/app/auth"
)

// SetupService creates the first admin account (ADR 0032 §6).
type SetupService interface {
	// SetupRequired reports whether no account exists yet.
	SetupRequired(ctx context.Context) (bool, error)
	// Setup creates the first admin and signs it in.
	Setup(ctx context.Context, request auth.SignInRequest) (auth.SignInResult, error)
}

// SignInService creates sessions and changes the password that opens them.
type SignInService interface {
	// SignIn checks name and password and creates a session.
	SignIn(ctx context.Context, request auth.SignInRequest) (auth.SignInResult, error)
	// ChangePassword replaces the password of the viewer's account.
	ChangePassword(ctx context.Context, change auth.PasswordChange) error
}

// ProfileSelectionService describes the signed-in account and selects its profile.
type ProfileSelectionService interface {
	// Me describes the session's account, selected profile and picker list.
	Me(ctx context.Context) (auth.Me, error)
	// SelectProfile selects a profile of the session's account.
	SelectProfile(ctx context.Context, selection auth.ProfileSelection) (auth.Me, error)
	// ClearProfile leaves the selected profile.
	ClearProfile(ctx context.Context) error
}

// OwnDeviceService lists and signs out the devices of the session's account.
type OwnDeviceService interface {
	// ListDevices returns the account's devices, most recently seen first.
	ListDevices(ctx context.Context) ([]auth.DeviceView, error)
	// RevokeOwnDevice signs a device out; signedOut reports that it was the caller's own.
	RevokeOwnDevice(ctx context.Context, deviceID string) (signedOut bool, err error)
}

// AuthService is what the setup, sign-in, me, profile and own-device endpoints need of
// app/auth.Service. Without it those endpoints answer 501 "not_implemented".
type AuthService interface {
	SetupService
	SignInService
	ProfileSelectionService
	OwnDeviceService
}

// Operation names of the account endpoints for wrapped errors.
const (
	operationGetSetupState     = "get setup state"
	operationCompleteSetup     = "complete setup"
	operationCreateSession     = "create session"
	operationDeleteSession     = "delete session"
	operationGetMe             = "get me"
	operationSelectProfile     = "select profile"
	operationClearProfile      = "clear profile selection"
	operationChangeOwnPassword = "change own password"
	operationListOwnDevices    = "list own devices"
	operationRevokeOwnDevice   = "revoke own device"
)

// authOperationIDs are the operations of api/openapi.yaml that AuthService serves, as the
// generated strict handlers name them.
func authOperationIDs() []string {
	return []string{
		"GetSetupState", "CompleteSetup", "CreateSession", "DeleteSession", "GetMe", "SelectProfile",
		"ClearProfileSelection", "ChangeOwnPassword", "ListOwnDevices", "RevokeOwnDevice",
	}
}

// clientRequest is what the account handlers need of the raw request, which the generated
// strict handlers do not pass on.
type clientRequest struct {
	// userAgent is only turned into a short device label; it is never stored or logged.
	userAgent string
	// secure is true for an https request as the HTTP server resolved it (a TLS connection,
	// or X-Forwarded-Proto of a trusted reverse proxy, ADR 0032 §3); the session cookie is
	// then marked Secure.
	secure bool
}

type (
	clientRequestKey struct{}
	peerKey          struct{}
)

// WithPeer stores the classified network origin of a request for the account handlers. The
// HTTP server calls it, because only it knows which addresses count as local.
func WithPeer(ctx context.Context, peer auth.Peer) context.Context {
	return context.WithValue(ctx, peerKey{}, peer)
}

// PeerFrom returns the peer stored by WithPeer. Without one, as in a server without the
// session middleware, the peer counts as remote: the stricter reading.
func PeerFrom(ctx context.Context) auth.Peer {
	if peer, ok := ctx.Value(peerKey{}).(auth.Peer); ok {
		return peer
	}

	return auth.Peer{Class: auth.AddressRemote}
}

func clientRequestFrom(ctx context.Context) clientRequest {
	client, _ := ctx.Value(clientRequestKey{}).(clientRequest)

	return client
}

// strictMiddlewares are the middlewares of the generated strict handlers.
func (api *API) strictMiddlewares() []apigen.StrictMiddlewareFunc {
	return []apigen.StrictMiddlewareFunc{api.serveAuthOperations}
}

// serveAuthOperations answers the account operations with 501 while no AuthService is
// wired, and otherwise hands their handlers the request facts they need. Other operations
// pass through untouched.
func (api *API) serveAuthOperations(next apigen.StrictHandlerFunc, operationID string) apigen.StrictHandlerFunc {
	if !slices.Contains(authOperationIDs(), operationID) {
		return next
	}

	return func(ctx context.Context, writer http.ResponseWriter, request *http.Request, body any) (any, error) {
		if api.auth == nil {
			WriteProblem(writer, notImplementedProblem())

			// The answer is written; a nil response tells the strict handler to add nothing.
			return nil, nil //nolint:nilnil // see above
		}

		// ctx is the request context the generated strict handler passes on.
		client := clientRequest{userAgent: request.UserAgent(), secure: clientorigin.From(ctx).Secure}

		return next(context.WithValue(ctx, clientRequestKey{}, client), writer, request, body)
	}
}
