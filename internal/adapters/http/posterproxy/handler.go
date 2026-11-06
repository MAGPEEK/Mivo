package posterproxy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/logtext"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/domain"
)

const (
	posterIDParameter = "posterId"
	// posterPattern serves one poster; GET also matches HEAD.
	posterPattern = http.MethodGet + " " + pathPrefix + "{" + posterIDParameter + "}"
	// cacheControl matches the ID lifetime: the browser may keep the bytes as long as the
	// server keeps the ID, and an ID never names another picture.
	cacheControl = "private, max-age=3600"
)

// servedTypes are the picture formats the route answers with (api/openapi.yaml); the
// artwork fetcher only lets these through.
func servedTypes() []string {
	return []string{"image/jpeg", "image/png", "image/webp"}
}

// Mounts returns the poster route for restapi.Options.Mounts.
func (proxy *Proxy) Mounts() []restapi.Mount {
	return []restapi.Mount{{Pattern: posterPattern, Handler: http.HandlerFunc(proxy.servePoster)}}
}

// QuietPatterns returns the pattern of the poster route for server.Options.QuietPatterns:
// a result list loads many previews at once.
func QuietPatterns() []string {
	return []string{posterPattern}
}

func (proxy *Proxy) servePoster(writer http.ResponseWriter, request *http.Request) {
	id := request.PathValue(posterIDParameter)
	if !isID(id) {
		restapi.WriteProblem(writer, restapi.NewProblem(http.StatusBadRequest, restapi.CodeInvalid))

		return
	}

	found, ok := proxy.lookup(id)
	if !ok {
		restapi.WriteProblem(writer, restapi.NewProblem(http.StatusNotFound, restapi.CodeNotFound))

		return
	}

	body, err := proxy.picture(request.Context(), id, found)
	if err != nil {
		proxy.writeFailure(writer, request, err)

		return
	}

	writePicture(writer, body)
}

// picture returns the held bytes of an entry or downloads them once. The entry's fetch lock
// covers only the download and keep, never the write to a (slow) client. A download that is
// already running when online metadata is switched off completes and its bytes are kept;
// only new downloads are refused.
func (proxy *Proxy) picture(ctx context.Context, id string, found *entry) ([]byte, error) {
	if err := found.lockFetch(ctx); err != nil {
		return nil, err
	}

	defer found.unlockFetch()

	if found.body != nil {
		return found.body, nil
	}

	settings, err := proxy.options.Settings.OnlineSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("read online settings: %w", err)
	}

	if !settings.Enabled || !settings.TMDB {
		return nil, fmt.Errorf("online metadata or tmdb is switched off: %w", domain.ErrConflict)
	}

	fetched, err := proxy.options.Source.Preview(ctx, found.ref)
	if err != nil {
		return nil, sourceError{fmt.Errorf("preview poster: %w", err)}
	}

	proxy.keep(id, found, fetched)

	return fetched, nil
}

// sourceError marks a failure of the provider download, the only one that is a provider
// outage (provider_unavailable with Retry-After); a failed settings read is not.
type sourceError struct{ error }

func (err sourceError) Unwrap() error { return err.error }

func isProviderOutage(err error) bool {
	var source sourceError

	return errors.As(err, &source) && (errors.Is(err, domain.ErrUnavailable) || errors.Is(err, domain.ErrRateLimited))
}

func writePicture(writer http.ResponseWriter, body []byte) {
	mediaType := http.DetectContentType(body)
	if !isServedType(mediaType) {
		// The fetcher validated the picture; a format this route does not declare is
		// treated like a missing picture rather than sent with a wrong type.
		restapi.WriteProblem(writer, restapi.NewProblem(http.StatusNotFound, restapi.CodeNotFound))

		return
	}

	header := writer.Header()
	header.Set("Content-Type", mediaType)
	header.Set("Content-Length", strconv.Itoa(len(body)))
	header.Set("Cache-Control", cacheControl)
	header.Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(http.StatusOK)

	// A failed write means the client has gone away; there is nobody left to tell.
	_, _ = writer.Write(body)
}

func isServedType(mediaType string) bool {
	for _, served := range servedTypes() {
		if mediaType == served {
			return true
		}
	}

	return false
}

// writeFailure maps a failed fetch: a picture the provider does not have, refuses or that is
// no valid image is not found; online metadata or the provider switched off is
// online_disabled (no outage, so no Retry-After); an unreachable or rate-limiting provider is
// provider_unavailable; anything else, such as a failed settings read, is logged and
// answered 500.
func (proxy *Proxy) writeFailure(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case request.Context().Err() != nil:
		// The client went away; nobody reads an answer.
		return
	case errors.Is(err, domain.ErrConflict):
		restapi.WriteProblem(writer, restapi.NewProblem(http.StatusConflict, restapi.CodeOnlineDisabled))
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, domain.ErrInvalid), errors.Is(err, domain.ErrForbidden):
		restapi.WriteProblem(writer, restapi.NewProblem(http.StatusNotFound, restapi.CodeNotFound))
	case isProviderOutage(err):
		writer.Header().Set("Retry-After", strconv.Itoa(int(restapi.ProviderRetryAfterSeconds)))
		restapi.WriteProblem(writer, unavailableProblem())
	default:
		proxy.options.Logger.ErrorContext(request.Context(), "poster preview failed",
			logtext.RequestAttributes(request, slog.Any("error", err))...)
		restapi.WriteProblem(writer, restapi.InternalProblem())
	}
}

func unavailableProblem() apigen.Problem {
	problem := restapi.NewProblem(http.StatusServiceUnavailable, restapi.CodeProviderUnavailable)
	seconds := restapi.ProviderRetryAfterSeconds
	problem.RetryAfterSeconds = &seconds

	return problem
}

// isID accepts the 32 lower-case hex digits newID produces.
func isID(text string) bool {
	if len(text) != 2*idBytes {
		return false
	}

	for index := range len(text) {
		char := text[index]
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}

	return true
}
