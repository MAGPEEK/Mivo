package stream

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/adapters/http/statuswriter"
	appsubtitles "github.com/magpeek/mivo/internal/app/subtitles"
	"github.com/magpeek/mivo/internal/domain"
)

// Subtitle delivery (ADR 0030 section 3). The rendition is cached per file and track, so a
// conversion is the only slow part and is bounded by conversionTimeout.
const (
	subtitleSuffix      = ".vtt"
	subtitleContentType = "text/vtt; charset=utf-8"
	subtitleCaching     = "private, max-age=3600"
	headerETag          = "ETag"
	// conversionTimeout is how long a request waits for a queued conversion before it answers
	// 503 with Retry-After.
	conversionTimeout = 20 * time.Second
	// conversionRetrySeconds is the Retry-After of that 503: the conversion keeps running.
	conversionRetrySeconds = 5
)

// subtitleTrackPattern is the track id shape of the contract (SubtitleTrack.id). Everything
// else is rejected before any lookup, so no odd id reaches the service or a file name.
var subtitleTrackPattern = regexp.MustCompile(`^(s:[0-9]{1,9}|x:[0-9a-f]{1,64})$`)

// Subtitles converts the subtitle tracks of media files to WebVTT. Implemented by
// *subtitles.Service (app/subtitles), which checks the viewer.
type Subtitles interface {
	// WebVTT returns the track as WebVTT; the caller closes Content. Errors wrap
	// domain.ErrUnauthenticated without a viewer, domain.ErrNotFound for an unknown or hidden
	// file or track, domain.ErrInvalid for a picture subtitle.
	WebVTT(ctx context.Context, fileID, trackID string) (appsubtitles.WebVTT, error)
}

// fileIDOf returns the canonical file id of the request, and false for anything that is no
// UUID. The routes answer 404 then, the same as for an unknown file.
func fileIDOf(request *http.Request) (string, bool) {
	parsed, err := uuid.Parse(request.PathValue(fileIDParameter))
	if err != nil {
		return "", false
	}

	return parsed.String(), true
}

// serveSubtitle serves a subtitle track (GET|HEAD /api/v1/media-files/{fileId}/subtitles/
// {trackId}.vtt) with ETag, conditional requests and Range.
func (handlers *Handlers) serveSubtitle(writer http.ResponseWriter, request *http.Request) {
	response := statuswriter.New(writer)

	trackID, trackOK := strings.CutSuffix(request.PathValue(trackFileParameter), subtitleSuffix)
	fileID, fileOK := fileIDOf(request)

	if !trackOK || !fileOK || !subtitleTrackPattern.MatchString(trackID) {
		writeProblem(response, notFoundProblem())

		return
	}

	ctx, cancel := context.WithTimeout(request.Context(), handlers.conversionTimeout)
	defer cancel()

	subtitle, err := handlers.subtitles.WebVTT(ctx, fileID, trackID)
	if err != nil {
		handlers.failSubtitle(response, request, err, ctx.Err() != nil && errors.Is(err, ctx.Err()))

		return
	}

	defer func() { _ = subtitle.Content.Close() }()

	header := response.Header()
	header.Set(headerContentType, subtitleContentType)
	header.Set(headerCacheControl, subtitleCaching)
	header.Set(headerETag, subtitleETag(subtitle))
	http.ServeContent(response, request, "", subtitle.ModTime, subtitle.Content)
}

// subtitleETag derives the validator from what the cache stamp changes: a converted
// rendition of a changed source is a new file with another size or time.
func subtitleETag(subtitle appsubtitles.WebVTT) string {
	return `"` + strconv.FormatInt(subtitle.Size, 16) + "-" + strconv.FormatInt(subtitle.ModTime.UnixNano(), 16) + `"`
}

// failSubtitle answers a failed subtitle request (errors are mapped only in this adapter,
// ADR 0017): a conversion that outlived conversionTimeout is 503 with Retry-After, a picture
// subtitle (ErrInvalid) or a path outside the library (ErrForbidden) is 422 unsupported, the
// rest follows failMedia.
func (handlers *Handlers) failSubtitle(response *statuswriter.Recorder, request *http.Request, err error, timedOut bool) {
	switch {
	case request.Context().Err() != nil:
		return // the client left; nobody reads an answer
	case timedOut:
		response.Header().Set(headerRetryAfter, strconv.Itoa(conversionRetrySeconds))
		writeProblem(response, restapi.NewProblem(http.StatusServiceUnavailable, restapi.CodeUnavailable))
	case errors.Is(err, domain.ErrInvalid), errors.Is(err, domain.ErrForbidden):
		writeProblem(response, restapi.NewProblem(http.StatusUnprocessableEntity, restapi.CodeUnsupported))
	default:
		handlers.failMedia(response, request, err)
	}
}

// failMedia answers a failed request of a media-file derivative with restapi.ProblemForError:
// 401 without a viewer, 404 for an unknown and a hidden file alike, 503 for unavailable
// storage. Only a 500 is logged; the cause stays out of the response.
func (handlers *Handlers) failMedia(response *statuswriter.Recorder, request *http.Request, err error) {
	if request.Context().Err() != nil {
		return // the client left; nobody reads an answer
	}

	problem := restapi.ProblemForError(err)
	if problem.Status == http.StatusInternalServerError {
		handlers.logger.ErrorContext(request.Context(), "serve media file derivative", "error", err)
	}

	writeProblem(response, problem)
}
