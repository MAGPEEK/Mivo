package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

const (
	setupPath   = "/api/v1/setup"
	signInPath  = "/api/v1/auth/session"
	profilePath = "/api/v1/me/profile"
	// smokeDeviceName names the session of the smoke in the device list of the account.
	smokeDeviceName = "playback-smoke"
	// generatedAdminName is the administrator the smoke creates on a server in setup state
	// when no -user is given.
	generatedAdminName = "playback-smoke"
)

var (
	errCredentialsNeeded = errors.New("the server has accounts already; pass -user and -password-env")
	errNoToken           = errors.New("the server returned no bearer token for client cli")
	errProfileNeedsPIN   = errors.New("the session has no profile and the primary profile has a PIN; sign in with an account whose primary profile has none")
	errNoPrimaryProfile  = errors.New("the session has no profile and the account lists no primary profile")
)

// credentials are the account the smoke signs in with; the password never comes from the
// command line.
type credentials struct {
	name     string
	password string
}

// authentication is how a run talks to the server: anonymous before sign-in existed
// (pre-M7 servers answer 404, the M7-01 stubs 501), else signed in with a bearer token.
type authentication struct {
	signedIn bool
	account  string
	// createdAdmin is true when the run performed the first-run setup.
	createdAdmin bool
}

func (auth authentication) String() string {
	switch {
	case !auth.signedIn:
		return "without sign-in (the server has no accounts)"
	case auth.createdAdmin:
		return fmt.Sprintf("signed in as %q, created by the first-run setup", auth.account)
	default:
		return fmt.Sprintf("signed in as %q", auth.account)
	}
}

// signIn authenticates the client: it runs the first-run setup on a fresh server, signs in
// with given on a server with accounts, or stays anonymous on a server without sign-in. On
// success the client carries the bearer token of the new session.
func signIn(ctx context.Context, client *apiClient, given *credentials) (authentication, error) {
	required, supported, err := setupRequired(ctx, client)
	if err != nil || !supported {
		return authentication{}, err
	}

	var (
		session apigen.Session
		created bool
	)

	switch {
	case required:
		session, err = completeSetup(ctx, client, given)
		created = true
	case given != nil:
		session, err = createSignInSession(ctx, client, *given)
	default:
		return authentication{}, errCredentialsNeeded
	}

	if err != nil {
		return authentication{}, err
	}

	if session.Token == nil || *session.Token == "" {
		return authentication{}, errNoToken
	}

	client.token = *session.Token

	if err := ensureProfile(ctx, client, session.Me); err != nil {
		return authentication{}, err
	}

	return authentication{signedIn: true, account: session.Me.Account.Name, createdAdmin: created}, nil
}

// setupRequired reads the setup state. supported is false on a server without the endpoint
// (404) or with the stub that answers 501 until the account handlers exist.
func setupRequired(ctx context.Context, client *apiClient) (required, supported bool, err error) {
	var state apigen.SetupState

	err = client.call(ctx, apiCall{method: http.MethodGet, reference: setupPath, want: http.StatusOK, out: &state})
	if problem, ok := errors.AsType[*problemError](err); ok &&
		(problem.status == http.StatusNotFound || problem.status == http.StatusNotImplemented) {
		return false, false, nil
	}

	if err != nil {
		return false, false, fmt.Errorf("read the setup state: %w", err)
	}

	return state.Required, true, nil
}

// completeSetup creates the first administrator, with given or a generated account. The
// generated password is never printed: later runs against the same server pass -user and
// -password-env, which the release gate does from the start.
func completeSetup(ctx context.Context, client *apiClient, given *credentials) (apigen.Session, error) {
	account := credentials{name: generatedAdminName}

	if given != nil {
		account = *given
	} else {
		// 26 characters of base32 (128 bits), well inside the 8 to 128 bytes the contract allows.
		account.password = rand.Text()
	}

	cli, deviceName := apigen.ClientKindCli, smokeDeviceName
	request := apigen.SetupRequest{Name: account.name, Password: &account.password, Client: &cli, DeviceName: &deviceName}

	var session apigen.Session

	err := client.call(ctx, apiCall{method: http.MethodPost, reference: setupPath, body: request, want: http.StatusCreated, out: &session})
	if err != nil {
		return apigen.Session{}, fmt.Errorf("complete the first-run setup as %q: %w", account.name, err)
	}

	return session, nil
}

func createSignInSession(ctx context.Context, client *apiClient, account credentials) (apigen.Session, error) {
	deviceName := smokeDeviceName
	request := apigen.SignInRequest{Name: account.name, Password: &account.password, Client: apigen.ClientKindCli, DeviceName: &deviceName}

	var session apigen.Session

	err := client.call(ctx, apiCall{method: http.MethodPost, reference: signInPath, body: request, want: http.StatusOK, out: &session})
	if err != nil {
		return apigen.Session{}, fmt.Errorf("sign in as %q: %w", account.name, err)
	}

	return session, nil
}

// ensureProfile selects the primary profile when the server did not select one (an account
// with managed profiles): catalog and playback answer 403 profile_required without one.
func ensureProfile(ctx context.Context, client *apiClient, me apigen.Me) error {
	if me.CurrentProfile != nil {
		return nil
	}

	for _, profile := range me.Profiles {
		if !profile.IsPrimary {
			continue
		}

		if profile.HasPin {
			return errProfileNeedsPIN
		}

		err := client.call(ctx, apiCall{
			method: http.MethodPut, reference: profilePath, body: apigen.ProfileSelection{ProfileID: profile.ID}, want: http.StatusOK,
		})
		if err != nil {
			return fmt.Errorf("select the primary profile: %w", err)
		}

		return nil
	}

	return errNoPrimaryProfile
}

// signOut ends the session, so the smoke leaves no device behind in the account.
func signOut(ctx context.Context, client *apiClient) error {
	if client.token == "" {
		return nil
	}

	err := client.call(ctx, apiCall{method: http.MethodDelete, reference: signInPath, want: http.StatusNoContent})
	if err != nil {
		return fmt.Errorf("sign out: %w", err)
	}

	client.token = ""

	return nil
}
