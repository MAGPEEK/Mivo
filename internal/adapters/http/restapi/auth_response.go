package restapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	appaccess "github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/app/auth"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

const (
	// sessionCookiePath sends the cookie with every request, the media URLs included.
	sessionCookiePath = "/"
	// sessionCookieMaxAgeSeconds is the absolute session lifetime; the server ends an idle
	// session earlier on its own.
	sessionCookieMaxAgeSeconds = int(auth.AbsoluteLifetime / time.Second)
	// expireCookieNow is the http.Cookie MaxAge that makes a browser drop the cookie.
	expireCookieNow = -1

	headerCacheControl = "Cache-Control"
	// valueNoStore keeps an answer that carries a session token out of every cache.
	valueNoStore = "no-store"
)

// sessionCookie is the browser session cookie of ADR 0032 §3. An empty token expires it.
func sessionCookie(token string, secure bool) *http.Cookie {
	// Secure follows the connection (ADR 0032 §3): a plain-HTTP home server could not use a
	// Secure cookie at all.
	cookie := &http.Cookie{ //nolint:gosec // see above; HttpOnly and SameSite are always set
		Name:     SessionCookieName,
		Value:    token,
		Path:     sessionCookiePath,
		MaxAge:   sessionCookieMaxAgeSeconds,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}

	if token == "" {
		cookie.MaxAge = expireCookieNow
	}

	return cookie
}

// signedInResponse answers setup and sign-in: the session cookie for a browser, the token in
// the body for every other client, never both.
type signedInResponse struct {
	status  int
	session apigen.Session
	// cookie is nil for clients that get the token in the body.
	cookie *http.Cookie
}

// newSignedInResponse describes the new session. The picker list comes from Me, read as the
// session that was just created; that viewer carries no library access, so it can do
// nothing else.
func (api *API) newSignedInResponse(ctx context.Context, result auth.SignInResult, status int) (signedInResponse, error) {
	viewer := access.Viewer{AuthSessionID: result.Session.ID, AccountID: result.Account.ID}
	if result.ProfileSelected {
		viewer.ProfileID = result.Profile.ID
	}

	me, err := api.auth.Me(appaccess.WithViewer(ctx, viewer))
	if err != nil {
		return signedInResponse{}, fmt.Errorf("describe new session: %w", &serverFault{cause: err})
	}

	described, err := meResponse(me)
	if err != nil {
		return signedInResponse{}, fmt.Errorf("describe new session: %w", &serverFault{cause: err})
	}

	response := signedInResponse{status: status, session: apigen.Session{Me: described}}

	if result.Session.Device.Client == auth.ClientWeb {
		response.cookie = sessionCookie(result.Token, clientRequestFrom(ctx).secure)
	} else {
		response.session.Token = &result.Token
	}

	return response, nil
}

// VisitCompleteSetupResponse writes the answer of completeSetup.
func (response signedInResponse) VisitCompleteSetupResponse(writer http.ResponseWriter) error {
	return response.write(writer)
}

// VisitCreateSessionResponse writes the answer of createSession.
func (response signedInResponse) VisitCreateSessionResponse(writer http.ResponseWriter) error {
	return response.write(writer)
}

func (response signedInResponse) write(writer http.ResponseWriter) error {
	body, err := json.Marshal(response.session)
	if err != nil {
		return fmt.Errorf("encode session: %w", err)
	}

	if response.cookie != nil {
		http.SetCookie(writer, response.cookie)
	}

	writer.Header().Set(headerCacheControl, valueNoStore)
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(response.status)

	_, err = writer.Write(body)
	if err != nil {
		return fmt.Errorf("write session: %w", err)
	}

	return nil
}

// signedOutResponse answers 204 and makes a browser drop the session cookie. Clients that
// use a bearer token ignore the header.
type signedOutResponse struct {
	secure bool
}

// VisitDeleteSessionResponse writes the answer of deleteSession.
func (response signedOutResponse) VisitDeleteSessionResponse(writer http.ResponseWriter) error {
	return response.write(writer)
}

// VisitRevokeOwnDeviceResponse writes the answer of revokeOwnDevice for the caller's own
// device.
func (response signedOutResponse) VisitRevokeOwnDeviceResponse(writer http.ResponseWriter) error {
	return response.write(writer)
}

func (response signedOutResponse) write(writer http.ResponseWriter) error {
	http.SetCookie(writer, sessionCookie("", response.secure))
	writer.WriteHeader(http.StatusNoContent)

	return nil
}

// meResponse converts the account, its selected profile and the picker list.
func meResponse(me auth.Me) (apigen.Me, error) {
	accountID, err := parseStoredID("account", me.Account.ID)
	if err != nil {
		return apigen.Me{}, err
	}

	response := apigen.Me{
		Account:  apigen.Account{ID: accountID, Name: me.Account.Name, Role: apigen.AccountRole(me.Account.Role)},
		Profiles: make([]apigen.ProfileSummary, 0, len(me.Profiles)),
	}

	for _, entry := range me.Profiles {
		summary, err := profileSummary(entry.Profile, entry.Admin)
		if err != nil {
			return apigen.Me{}, err
		}

		response.Profiles = append(response.Profiles, summary)
	}

	if me.ProfileSelected {
		current, err := profileSummary(me.Profile, account.IsAdminViewer(me.Account, me.Profile))
		if err != nil {
			return apigen.Me{}, err
		}

		response.CurrentProfile = &current
	}

	return response, nil
}

func profileSummary(profile account.Profile, admin bool) (apigen.ProfileSummary, error) {
	id, err := parseStoredID("profile", profile.ID)
	if err != nil {
		return apigen.ProfileSummary{}, err
	}

	return apigen.ProfileSummary{
		ID:          id,
		Name:        profile.Name,
		AvatarColor: apigen.AvatarColor(profile.AvatarColor),
		HasPin:      profile.HasPIN,
		IsAdmin:     admin,
		IsPrimary:   profile.Primary,
	}, nil
}

// deviceListResponse converts the devices. A device is built field by field from
// auth.DeviceView, so nothing of the session row beyond these fields (BACKLOG m7-09: the
// token hash) can reach a client.
func deviceListResponse(devices []auth.DeviceView) (apigen.DeviceList, error) {
	response := apigen.DeviceList{Devices: make([]apigen.Device, 0, len(devices))}

	for _, device := range devices {
		id, err := parseStoredID("device", device.ID)
		if err != nil {
			return apigen.DeviceList{}, err
		}

		entry := apigen.Device{
			ID:           id,
			Name:         device.Device.Name,
			Client:       apigen.ClientKind(device.Device.Client),
			LastSeenAt:   device.LastSeenAt.UTC(),
			AddressClass: apigen.AddressClass(device.AddressClass),
			Current:      device.Current,
		}

		if device.Device.Platform != "" {
			platform := device.Device.Platform
			entry.Platform = &platform
		}

		response.Devices = append(response.Devices, entry)
	}

	return response, nil
}
