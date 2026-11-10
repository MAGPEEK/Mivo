package restapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	appaccess "github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/app/auth"
	"github.com/magpeek/mivo/internal/domain"
)

// GetSetupState reports whether the server still waits for its first account.
func (api *API) GetSetupState(ctx context.Context, _ apigen.GetSetupStateRequestObject) (apigen.GetSetupStateResponseObject, error) {
	required, err := api.auth.SetupRequired(ctx)
	if err != nil {
		return nil, operationFailure(operationGetSetupState, err)
	}

	return apigen.GetSetupState200JSONResponse{Required: required}, nil
}

// CompleteSetup creates the first admin and signs it in. The setup conflict is answered
// before the body is validated (BACKLOG m7-30): the web app's remote page probes with an
// empty setup request and must learn "conflict", not "invalid".
func (api *API) CompleteSetup(ctx context.Context, request apigen.CompleteSetupRequestObject) (apigen.CompleteSetupResponseObject, error) {
	required, err := api.auth.SetupRequired(ctx)
	if err != nil {
		return nil, operationFailure(operationCompleteSetup, err)
	}

	if !required {
		return nil, fmt.Errorf("%s: %w", operationCompleteSetup, auth.ErrSetupComplete)
	}

	if request.Body == nil {
		return nil, fmt.Errorf("%s: missing request body: %w", operationCompleteSetup, domain.ErrInvalid)
	}

	client := auth.ClientWeb
	if request.Body.Client != nil {
		client = auth.Client(*request.Body.Client)
	}

	result, err := api.auth.Setup(ctx, signInRequest(ctx, signInForm{
		name: request.Body.Name, password: request.Body.Password, client: client, deviceName: request.Body.DeviceName,
	}))
	if err != nil {
		return nil, operationFailure(operationCompleteSetup, err, http.StatusBadRequest, http.StatusConflict)
	}

	return api.newSignedInResponse(ctx, result, http.StatusCreated)
}

// CreateSession signs in with name and password.
func (api *API) CreateSession(ctx context.Context, request apigen.CreateSessionRequestObject) (apigen.CreateSessionResponseObject, error) {
	if request.Body == nil {
		return nil, fmt.Errorf("%s: missing request body: %w", operationCreateSession, domain.ErrInvalid)
	}

	result, err := api.auth.SignIn(ctx, signInRequest(ctx, signInForm{
		name: request.Body.Name, password: request.Body.Password,
		client: auth.Client(request.Body.Client), deviceName: request.Body.DeviceName,
	}))
	if err != nil {
		return nil, operationFailure(operationCreateSession, err,
			http.StatusBadRequest, http.StatusUnauthorized, http.StatusTooManyRequests)
	}

	return api.newSignedInResponse(ctx, result, http.StatusOK)
}

// DeleteSession signs the caller out: its session, which is its device, is deleted and a
// browser's cookie cleared. A session that is gone already is signed out as well.
func (api *API) DeleteSession(ctx context.Context, _ apigen.DeleteSessionRequestObject) (apigen.DeleteSessionResponseObject, error) {
	viewer, ok := appaccess.ViewerFrom(ctx)
	if !ok || viewer.AuthSessionID == "" {
		return nil, fmt.Errorf("%s: %w", operationDeleteSession, domain.ErrUnauthenticated)
	}

	_, err := api.auth.RevokeOwnDevice(ctx, viewer.AuthSessionID)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, operationFailure(operationDeleteSession, err, http.StatusUnauthorized)
	}

	return signedOutResponse{secure: clientRequestFrom(ctx).secure}, nil
}

// signInForm is what setup and sign-in read from their bodies.
type signInForm struct {
	name     string
	password *string
	client   auth.Client
	// deviceName is the name the client gave its device, if any.
	deviceName *string
}

// signInRequest builds the use-case request. A missing device name becomes a short label
// derived from the User-Agent; the raw header never leaves this adapter.
func signInRequest(ctx context.Context, form signInForm) auth.SignInRequest {
	label := labelUserAgent(clientRequestFrom(ctx).userAgent)
	name := strings.TrimSpace(textOrEmpty(form.deviceName))

	if name == "" {
		name = label.name()
	}

	return auth.SignInRequest{
		Name:     form.name,
		Password: textOrEmpty(form.password),
		Device:   auth.Device{Name: name, Client: form.client, Platform: label.platform},
		Peer:     PeerFrom(ctx),
	}
}

func textOrEmpty(text *string) string {
	if text == nil {
		return ""
	}

	return *text
}
