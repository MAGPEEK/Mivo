package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

const fakeToken = "fake-bearer-token" //nolint:gosec // a test value of the fake server, no credential

// fakeAuth is the account part of the fake server (ADR 0032): the setup state, sign-in,
// profile selection, sign-out and the bearer check of every other request.
type fakeAuth struct {
	// stubStatus, when set, answers every account endpoint with it and checks no token,
	// like the 501 stubs of M7-01.
	stubStatus int
	// accounts maps names to passwords; empty means setup state.
	accounts map[string]string
	// managedProfiles leaves the session without a selected profile after sign-in.
	managedProfiles bool
	primaryHasPIN   bool
	// noPrimary lists no primary profile and selects none.
	noPrimary bool
	// profileStatus, when set, answers the profile selection with it.
	profileStatus int
	// noToken makes setup and sign-in answer without a token.
	noToken bool

	token           string
	profileSelected bool
	setupRequest    apigen.SetupRequest
	signInRequest   apigen.SignInRequest
	setups          int
	signIns         int
	signOuts        int
	// rejected lists the requests that came without the right token or profile.
	rejected []string
}

func (fake *fakeServer) authRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/setup", fake.setupState)
	mux.HandleFunc("POST /api/v1/setup", fake.completeSetup)
	mux.HandleFunc("POST /api/v1/auth/session", fake.signIn)
	mux.HandleFunc("DELETE /api/v1/auth/session", fake.signOut)
	mux.HandleFunc("PUT /api/v1/me/profile", fake.selectProfile)
}

// requireSession lets through the public operations and answers every other request
// without the bearer token with 401, and without a profile with 403, like the server.
func (fake *fakeServer) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		status, code := fake.sessionProblem(r)
		fake.mu.Unlock()

		if status != 0 {
			fake.problem(w, status, code, 0)

			return
		}

		next.ServeHTTP(w, r)
	})
}

func (fake *fakeServer) sessionProblem(r *http.Request) (status int, code string) {
	auth := fake.auth
	if auth == nil || auth.stubStatus != 0 || isPublic(r) {
		return 0, ""
	}

	if auth.token == "" || r.Header.Get("Authorization") != "Bearer "+auth.token {
		auth.rejected = append(auth.rejected, r.Method+" "+r.URL.Path)

		return http.StatusUnauthorized, "unauthenticated"
	}

	if !auth.profileSelected && !strings.HasPrefix(r.URL.Path, "/api/v1/me") && !strings.HasPrefix(r.URL.Path, "/api/v1/auth") {
		auth.rejected = append(auth.rejected, r.Method+" "+r.URL.Path)

		return http.StatusForbidden, "profile_required"
	}

	return 0, ""
}

func isPublic(r *http.Request) bool {
	switch r.URL.Path {
	case "/api/v1/system/info", "/api/v1/setup":
		return true
	case "/api/v1/auth/session":
		return r.Method == http.MethodPost
	default:
		return false
	}
}

// accountEndpoint answers like a server before sign-in (404) or with the stubs (501) and
// reports whether the account endpoints are live.
func (fake *fakeServer) accountEndpoint(w http.ResponseWriter) bool {
	switch {
	case fake.auth == nil:
		fake.problem(w, http.StatusNotFound, "not_found", 0)
	case fake.auth.stubStatus != 0:
		fake.problem(w, fake.auth.stubStatus, "not_implemented", 0)
	default:
		return true
	}

	return false
}

func (fake *fakeServer) setupState(w http.ResponseWriter, _ *http.Request) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	if fake.accountEndpoint(w) {
		fake.writeJSON(w, http.StatusOK, apigen.SetupState{Required: len(fake.auth.accounts) == 0})
	}
}

func (fake *fakeServer) completeSetup(w http.ResponseWriter, r *http.Request) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	if !fake.accountEndpoint(w) {
		return
	}

	auth := fake.auth
	if len(auth.accounts) > 0 {
		fake.problem(w, http.StatusConflict, "conflict", 0)

		return
	}

	if err := json.NewDecoder(r.Body).Decode(&auth.setupRequest); err != nil || auth.setupRequest.Password == nil {
		fake.problem(w, http.StatusBadRequest, "invalid", 0)

		return
	}

	auth.setups++
	auth.accounts = map[string]string{auth.setupRequest.Name: *auth.setupRequest.Password}
	fake.writeJSON(w, http.StatusCreated, fake.newSession(auth.setupRequest.Name))
}

func (fake *fakeServer) signIn(w http.ResponseWriter, r *http.Request) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	if !fake.accountEndpoint(w) {
		return
	}

	auth := fake.auth
	if err := json.NewDecoder(r.Body).Decode(&auth.signInRequest); err != nil || auth.signInRequest.Password == nil {
		fake.problem(w, http.StatusBadRequest, "invalid", 0)

		return
	}

	password, known := auth.accounts[auth.signInRequest.Name]
	if !known || password != *auth.signInRequest.Password {
		fake.problem(w, http.StatusUnauthorized, "invalid_credentials", 0)

		return
	}

	auth.signIns++
	fake.writeJSON(w, http.StatusOK, fake.newSession(auth.signInRequest.Name))
}

// newSession signs the account in; the primary profile is selected unless the account has
// managed profiles or the primary profile has a PIN, as in ADR 0032.
func (fake *fakeServer) newSession(name string) apigen.Session {
	auth := fake.auth
	auth.token = fakeToken
	primary := apigen.ProfileSummary{ID: uuid.New(), Name: name, IsPrimary: !auth.noPrimary, IsAdmin: true, HasPin: auth.primaryHasPIN}
	me := apigen.Me{
		Account:  apigen.Account{ID: uuid.New(), Name: name, Role: apigen.AccountRoleAdmin},
		Profiles: []apigen.ProfileSummary{primary, {ID: uuid.New(), Name: "Kid"}},
	}

	auth.profileSelected = !auth.managedProfiles && !auth.primaryHasPIN && !auth.noPrimary
	if auth.profileSelected {
		me.CurrentProfile = &primary
	}

	session := apigen.Session{Me: me}
	if !auth.noToken {
		token := auth.token
		session.Token = &token
	}

	return session
}

func (fake *fakeServer) selectProfile(w http.ResponseWriter, r *http.Request) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	var selection apigen.ProfileSelection
	if err := json.NewDecoder(r.Body).Decode(&selection); err != nil || selection.ProfileID == (uuid.UUID{}) {
		fake.problem(w, http.StatusBadRequest, "invalid", 0)

		return
	}

	if fake.auth.profileStatus != 0 {
		fake.problem(w, fake.auth.profileStatus, "internal", 0)

		return
	}

	fake.auth.profileSelected = true
	fake.writeJSON(w, http.StatusOK, apigen.Me{})
}

func (fake *fakeServer) signOut(w http.ResponseWriter, _ *http.Request) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	if !fake.accountEndpoint(w) {
		return
	}

	fake.auth.signOuts++
	fake.auth.token = ""
	w.WriteHeader(http.StatusNoContent)
}
