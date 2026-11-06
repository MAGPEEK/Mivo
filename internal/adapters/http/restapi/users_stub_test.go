package restapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
)

// TestUserStubsAnswerNotImplemented: every user administration operation of the M7-02
// contract is routed and answers 501 "not_implemented" until M7-22 serves it.
func TestUserStubsAnswerNotImplemented(t *testing.T) {
	t.Parallel()

	const (
		user    = "/api/v1/users/1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d"
		profile = "/api/v1/profiles/6f5e4d3c-2b1a-4f0e-9d8c-7b6a5f4e3d2c"
		device  = "/api/v1/devices/8e7d6c5b-4a3f-4e2d-9c1b-0a9f8e7d6c5b"
	)

	tests := []struct {
		method string
		target string
		body   string
	}{
		{method: http.MethodGet, target: "/api/v1/users"},
		{method: http.MethodPost, target: "/api/v1/users", body: `{"name":"Ben","password":"long enough","role":"user"}`},
		{method: http.MethodGet, target: user},
		{method: http.MethodPatch, target: user, body: `{"status":"disabled"}`},
		{method: http.MethodDelete, target: user},
		{method: http.MethodPut, target: user + "/password", body: `{"password":"long enough"}`},
		{method: http.MethodGet, target: user + "/profiles"},
		{method: http.MethodPost, target: user + "/profiles", body: `{"name":"Mia","pin":"1234"}`},
		{method: http.MethodPatch, target: profile, body: `{"pin":null,"contentLimit":{"preset":"adults","maxAge":null,"hideUnrated":true}}`},
		{method: http.MethodDelete, target: profile},
		{method: http.MethodGet, target: "/api/v1/devices"},
		{method: http.MethodDelete, target: device},
	}

	handler, _ := newTestHandlerWithOptions(t, restapi.Options{})

	for _, test := range tests {
		t.Run(test.method+" "+test.target, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequestWithContext(t.Context(), test.method, test.target, strings.NewReader(test.body))
			if test.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}

			assertProblem(t, serveRequest(t, handler, request), http.StatusNotImplemented, "not_implemented")
		})
	}
}

// TestProfileUpdateKeepsOmittedPINApartFromNull pins the generated type M7-22 relies on: an
// omitted pin keeps the PIN, null removes it and a string sets it.
func TestProfileUpdateKeepsOmittedPINApartFromNull(t *testing.T) {
	t.Parallel()

	omitted := decode[apigen.ProfileUpdate](t, []byte(`{}`))
	if omitted.Pin.IsSpecified() {
		t.Error("omitted pin is specified, want it unspecified")
	}

	cleared := decode[apigen.ProfileUpdate](t, []byte(`{"pin":null}`))
	if !cleared.Pin.IsSpecified() || !cleared.Pin.IsNull() {
		t.Error("pin null is not a specified null")
	}

	set := decode[apigen.ProfileUpdate](t, []byte(`{"pin":"1234"}`))
	if pin, err := set.Pin.Get(); err != nil || pin != "1234" {
		t.Errorf("pin = %q, %v, want 1234", pin, err)
	}
}
