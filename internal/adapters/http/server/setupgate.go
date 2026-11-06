package server

import (
	"log/slog"
	"net/http"
	"path"

	"github.com/magpeek/mivo/internal/adapters/http/logtext"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/app/auth"
)

// setupGate answers a setup write with 409 conflict once an account exists, before the
// handler parses the body (BACKLOG m7-30): the web app's remote page probes with an empty
// setup request and must learn "conflict", not "invalid". It runs after the management
// guard, so a remote or cross-origin peer still gets 403 forbidden first. The use case
// refuses a late setup in its transaction as well; this check only orders the answers.
type setupGate struct {
	logger *slog.Logger
	setup  SetupStateReader
}

func (gate setupGate) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !isSetupWrite(request) {
			next.ServeHTTP(writer, request)

			return
		}

		required, err := gate.setup.SetupRequired(request.Context())
		if err != nil {
			gate.logger.ErrorContext(request.Context(), "read setup state",
				logtext.RequestAttributes(request, slog.Any("error", err))...)
			restapi.WriteProblem(writer, restapi.InternalProblem())

			return
		}

		if !required {
			restapi.WriteProblem(writer, restapi.ProblemForError(auth.ErrSetupComplete))

			return
		}

		next.ServeHTTP(writer, request)
	})
}

// isSetupWrite reports a POST whose decoded, cleaned path is the setup route, so every
// spelling the router could serve as completeSetup is gated.
func isSetupWrite(request *http.Request) bool {
	return request.Method == http.MethodPost && path.Clean("/"+request.URL.Path) == apiV1Setup
}
