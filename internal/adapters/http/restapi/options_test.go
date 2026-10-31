package restapi_test

import (
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/domain"
)

const mountPattern = "GET /api/v1/test/{name}"

func TestNewHandlerRejectsInvalidOptions(t *testing.T) {
	t.Parallel()

	logger, _ := newTestLogger()
	mounted := http.NotFoundHandler()
	complete := completeOptions(restapi.Options{Logger: logger})

	withMounts := func(mounts ...restapi.Mount) restapi.Options {
		options := complete
		options.Mounts = mounts

		return options
	}

	without := func(drop func(options *restapi.Options)) restapi.Options {
		options := complete
		drop(&options)

		return options
	}

	tests := []struct {
		name    string
		options restapi.Options
	}{
		{name: "missing system service", options: without(func(options *restapi.Options) { options.System = nil })},
		{name: "missing library catalog", options: without(func(options *restapi.Options) { options.Libraries = nil })},
		{name: "missing item catalog", options: without(func(options *restapi.Options) { options.Items = nil })},
		{name: "missing scan service", options: without(func(options *restapi.Options) { options.Scan = nil })},
		{name: "missing playback service", options: without(func(options *restapi.Options) { options.Playback = nil })},
		{name: "missing logger", options: without(func(options *restapi.Options) { options.Logger = nil })},
		{name: "nothing wired", options: restapi.Options{}},
		{name: "mount without pattern", options: withMounts(restapi.Mount{Handler: mounted})},
		{name: "mount without handler", options: withMounts(restapi.Mount{Pattern: mountPattern})},
		{name: "malformed mount pattern", options: withMounts(restapi.Mount{Pattern: "GET /api/v1/{", Handler: mounted})},
		{name: "mount outside the API", options: withMounts(restapi.Mount{Pattern: "GET /stream/{name}", Handler: mounted})},
		{name: "mount on the API prefix without its slash", options: withMounts(restapi.Mount{Pattern: "GET /apix/{name}", Handler: mounted})},
		{name: "mount with a host", options: withMounts(restapi.Mount{Pattern: "GET media.test/api/v1/x", Handler: mounted})},
		{name: "mount without method outside the API", options: withMounts(restapi.Mount{Pattern: "/{name}", Handler: mounted})},
		{name: "mount on a route of the contract", options: withMounts(restapi.Mount{Pattern: "GET " + healthPath, Handler: mounted})},
		{
			name: "two mounts on one route",
			options: withMounts(
				restapi.Mount{Pattern: mountPattern, Handler: mounted},
				restapi.Mount{Pattern: mountPattern, Handler: mounted},
			),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler, err := restapi.NewHandler(test.options)
			if !errors.Is(err, domain.ErrInvalid) || handler != nil {
				t.Errorf("NewHandler = %v, %v, want nil handler and ErrInvalid", handler, err)
			}
		})
	}
}

func TestMountPatternForms(t *testing.T) {
	t.Parallel()

	for _, pattern := range []string{"/api/v1/test/plain", "GET /api/v1/test/get", "HEAD  /api/v1/test/spaces"} {
		t.Run(pattern, func(t *testing.T) {
			t.Parallel()

			logger, _ := newTestLogger()

			_, err := restapi.NewHandler(completeOptions(restapi.Options{
				Logger: logger,
				Mounts: []restapi.Mount{{Pattern: pattern, Handler: http.NotFoundHandler()}},
			}))
			if err != nil {
				t.Errorf("NewHandler with mount %q: %v", pattern, err)
			}
		})
	}
}

func TestMountsAreServedWithTheAPIRouting(t *testing.T) {
	t.Parallel()

	// The mount sees its own path values: it answers only for the name the test requests.
	mounted := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.PathValue("name") != "abc" {
			writer.WriteHeader(http.StatusTeapot)

			return
		}

		writer.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(writer, "mounted abc")
	})

	handler, _ := newTestHandlerWithOptions(t, restapi.Options{
		Mounts: []restapi.Mount{{Pattern: mountPattern, Handler: mounted}},
	})

	t.Run("matching request", func(t *testing.T) {
		t.Parallel()

		got := serve(t, handler, http.MethodGet, "/api/v1/test/abc")
		if got.status != http.StatusOK || string(got.body) != "mounted abc" {
			t.Errorf("GET mount = %d %q, want 200 %q", got.status, got.body, "mounted abc")
		}
	})

	t.Run("contract routes still work", func(t *testing.T) {
		t.Parallel()

		if got := serve(t, handler, http.MethodGet, healthPath); got.status != http.StatusOK {
			t.Errorf("GET %s status = %d, want 200", healthPath, got.status)
		}
	})

	t.Run("wrong method", func(t *testing.T) {
		t.Parallel()

		got := serve(t, handler, http.MethodPost, "/api/v1/test/abc")
		assertProblem(t, got, http.StatusMethodNotAllowed, restapi.CodeMethodNotAllowed)

		if allow := got.header.Get("Allow"); allow != "GET, HEAD" {
			t.Errorf("Allow = %q, want %q", allow, "GET, HEAD")
		}
	})

	t.Run("path below the mount", func(t *testing.T) {
		t.Parallel()

		got := serve(t, handler, http.MethodGet, "/api/v1/test/abc/def")
		assertProblem(t, got, http.StatusNotFound, restapi.CodeNotFound)
	})
}
