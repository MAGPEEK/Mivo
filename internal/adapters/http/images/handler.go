package images

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/http/logtext"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/adapters/http/statuswriter"
	appimages "github.com/magpeek/mivo/internal/app/images"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

const (
	widthParameter = "width"
	tagParameter   = "tag"
	// maxTagLength is the maxLength of the tag parameter in api/openapi.yaml.
	maxTagLength = 64
	// unavailableRetrySeconds is the Retry-After of a 503: a render slot or the library
	// folder may be free again shortly.
	unavailableRetrySeconds = 5

	cacheImmutable  = "private, max-age=31536000, immutable"
	cacheRevalidate = "private, no-cache"
)

// Images renders artwork. Implemented by *images.Service (app/images).
type Images interface {
	// Image returns the artwork in the requested width. Errors wrap domain.ErrNotFound,
	// domain.ErrUnsupported or domain.ErrUnavailable.
	Image(ctx context.Context, request appimages.Request) (appimages.Image, error)
}

// Options are the dependencies of the image handler; both are required.
type Options struct {
	Images Images
	Logger *slog.Logger
}

// Handlers serves the image route. It is safe for concurrent use.
type Handlers struct {
	images Images
	logger *slog.Logger
}

// New returns the image handlers. A missing dependency is a wiring bug and wraps
// domain.ErrInvalid.
func New(options Options) (*Handlers, error) {
	switch {
	case options.Images == nil:
		return nil, fmt.Errorf("new image handlers: missing image service: %w", domain.ErrInvalid)
	case options.Logger == nil:
		return nil, fmt.Errorf("new image handlers: missing logger: %w", domain.ErrInvalid)
	}

	return &Handlers{images: options.Images, logger: options.Logger}, nil
}

// Mounts returns the image route for restapi.Options.Mounts.
func (handlers *Handlers) Mounts() []restapi.Mount {
	return []restapi.Mount{{Pattern: imagePattern, Handler: http.HandlerFunc(handlers.serveImage)}}
}

func (handlers *Handlers) serveImage(writer http.ResponseWriter, request *http.Request) {
	response := statuswriter.New(writer)

	parsed, ok := parseRequest(request)
	if !ok {
		restapi.WriteProblem(response, restapi.NewProblem(http.StatusBadRequest, restapi.CodeInvalid))

		return
	}

	image, err := handlers.images.Image(request.Context(), parsed)
	if err != nil {
		handlers.fail(response, request, err)

		return
	}

	defer func() { _ = image.Content.Close() }()

	header := response.Header()
	header.Set("Content-Type", image.ContentType)
	header.Set("ETag", image.ETag)
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Cache-Control", cacheControl(image))

	// Size is not passed: ServeContent measures the body by seeking, which also keeps it
	// right when the cached file changed length after Image stat'ed it.
	http.ServeContent(response, request, "", time.Time{}, image.Content)
}

func cacheControl(image appimages.Image) string {
	if image.Immutable {
		return cacheImmutable
	}

	return cacheRevalidate
}

// parseRequest validates the path and query. Values are never echoed back to the client.
func parseRequest(request *http.Request) (appimages.Request, bool) {
	id, err := uuid.Parse(request.PathValue(imageIDParameter))
	if err != nil {
		return appimages.Request{}, false
	}

	query := request.URL.Query()

	width, err := artwork.ParseWidth(query.Get(widthParameter))
	if err != nil {
		return appimages.Request{}, false
	}

	tag := query.Get(tagParameter)
	if !validTag(tag) {
		return appimages.Request{}, false
	}

	return appimages.Request{ID: id.String(), Width: width, Tag: tag}, true
}

// validTag accepts an empty tag (none sent) or up to maxTagLength lowercase hex digits.
func validTag(tag string) bool {
	if len(tag) > maxTagLength {
		return false
	}

	for _, character := range tag {
		isDigit := character >= '0' && character <= '9'
		isLowerHex := character >= 'a' && character <= 'f'

		if !isDigit && !isLowerHex {
			return false
		}
	}

	return true
}

// fail answers a failed image request. A client that went away gets nothing. ErrInvalid is
// deliberately not a 400: the request was validated above, so it can only describe stored
// data and is the server's fault.
func (handlers *Handlers) fail(response *statuswriter.Recorder, request *http.Request, err error) {
	if request.Context().Err() != nil {
		return
	}

	switch {
	case errors.Is(err, domain.ErrNotFound):
		restapi.WriteProblem(response, restapi.NewProblem(http.StatusNotFound, restapi.CodeNotFound))
	case errors.Is(err, domain.ErrUnsupported):
		restapi.WriteProblem(response, restapi.NewProblem(http.StatusUnprocessableEntity, restapi.CodeUnsupported))
	case errors.Is(err, domain.ErrUnavailable):
		response.Header().Set("Retry-After", strconv.Itoa(unavailableRetrySeconds))
		restapi.WriteProblem(response, restapi.NewProblem(http.StatusServiceUnavailable, restapi.CodeUnavailable))
	default:
		handlers.logger.ErrorContext(request.Context(), "serve image",
			logtext.RequestAttributes(request, logtext.Error(err))...)
		restapi.WriteProblem(response, restapi.InternalProblem())
	}
}
