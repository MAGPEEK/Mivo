package server

import (
	"errors"
	"net/http"
	"net/netip"
	"testing"

	appaccess "github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/domain"
	domainaccess "github.com/magpeek/mivo/internal/domain/access"
)

// viewerCapture remembers the viewer of the request it served.
type viewerCapture struct {
	viewer domainaccess.Viewer
	found  bool
}

func (capture *viewerCapture) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	capture.viewer, capture.found = appaccess.ViewerFrom(request.Context())

	writer.WriteHeader(http.StatusOK)
}

func captureOptions(t *testing.T, capture *viewerCapture, authentication Authentication) Options {
	t.Helper()

	logger, _ := newDebugLogger()

	return Options{
		Logger: logger, APIHandler: capture, Fallback: textHandler(http.StatusOK, "web"),
		InterfacePrefixes: &fakePrefixProvider{prefixes: []netip.Prefix{netip.MustParsePrefix(onLinkPrefix)}},
		Authentication:    authentication,
	}
}

func TestPreAccountModeServesAnUnrestrictedAdministrator(t *testing.T) {
	t.Parallel()

	capture := &viewerCapture{}
	request := newAuthRequest(t, authCase{method: http.MethodGet, target: itemsPath})

	if recorder := serveChain(t, captureOptions(t, capture, Authentication{}), request); recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}

	if !capture.found || !capture.viewer.Admin || capture.viewer.AuthSessionID != "" {
		t.Fatalf("viewer = %+v (found %v), want the administrator without a session", capture.viewer, capture.found)
	}

	if !capture.viewer.Restriction().IsUnrestricted() {
		t.Error("the pre-account viewer must read unrestricted")
	}
}

func TestWithAuthenticationAnUnauthenticatedRequestCarriesNoViewer(t *testing.T) {
	t.Parallel()

	capture := &viewerCapture{}
	options := captureOptions(t, capture, Authentication{Sessions: &fakeResolver{}, Setup: fakeSetup{}})
	request := newAuthRequest(t, authCase{method: http.MethodGet, target: itemsPath})

	if recorder := serveChain(t, options, request); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}

	if capture.found {
		t.Fatal("a refused request must not reach the handler with a viewer")
	}

	// Were a handler reached without a viewer, the use cases must still refuse.
	if _, err := appaccess.RestrictionFrom(request.Context()); err == nil || !isUnauthenticated(err) {
		t.Errorf("RestrictionFrom without viewer = %v, want ErrUnauthenticated", err)
	}
}

func isUnauthenticated(err error) bool {
	return err != nil && errors.Is(err, domain.ErrUnauthenticated)
}
