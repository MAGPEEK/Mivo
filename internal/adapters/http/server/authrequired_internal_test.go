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

// wiredAuthentication is a complete Authentication for tests that need New to accept the
// options but never reach a protected route.
func wiredAuthentication() Authentication {
	return Authentication{Sessions: &fakeResolver{}, Setup: fakeSetup{}}
}

func TestNewRefusesOptionsWithoutAuthentication(t *testing.T) {
	t.Parallel()

	logger, _ := newDebugLogger()

	_, err := New(Options{Address: "127.0.0.1:0", Logger: logger, APIHandler: viewerEcho()})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("New without Authentication = %v, want an error wrapping ErrInvalid (fail closed)", err)
	}
}

func TestWithAuthenticationAnUnauthenticatedRequestCarriesNoViewer(t *testing.T) {
	t.Parallel()

	capture := &viewerCapture{}
	options := captureOptions(t, capture, wiredAuthentication())
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

// TestNoRequestCarriesAViewerWithoutASession is the removal of the pre-account mode: whatever
// the route, an unauthenticated request never reaches a handler with a viewer, in setup state
// and after it, also for a bogus credential.
func TestNoRequestCarriesAViewerWithoutASession(t *testing.T) {
	t.Parallel()

	targets := []string{itemsPath, "/api/v1/libraries", "/api/v1/users", "/api/v1/playback/sessions"}
	setups := map[string]fakeSetup{"setup state": {required: true}, "after setup": {}}
	credentials := map[string]authCase{
		"no credential": {},
		"bogus bearer":  {bearer: "not-a-token"},
		"bogus cookie":  {cookie: "not-a-token"},
	}

	for setupName, setup := range setups {
		for credentialName, credential := range credentials {
			for _, target := range targets {
				t.Run(setupName+"/"+credentialName+target, func(t *testing.T) {
					t.Parallel()

					capture := &viewerCapture{}
					options := captureOptions(t, capture, Authentication{Sessions: &fakeResolver{}, Setup: setup})

					// A copy per subtest: the credential case is shared by the parallel ones.
					call := credential
					call.method, call.target = http.MethodGet, target

					recorder := serveChain(t, options, newAuthRequest(t, call))
					if recorder.Code != http.StatusUnauthorized || capture.found {
						t.Errorf("status = %d, viewer reached the handler = %v, want 401 and no viewer", recorder.Code, capture.found)
					}
				})
			}
		}
	}
}

func isUnauthenticated(err error) bool {
	return err != nil && errors.Is(err, domain.ErrUnauthenticated)
}
