package stream

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/magpeek/mivo/internal/adapters/http/logtext"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/adapters/http/statuswriter"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// HLS output names of ADR 0024 (FFmpeg's hls muxer writes them, ffmpeg.HLSArguments).
const (
	playlistName  = "index.m3u8"
	initName      = "init.mp4"
	segmentPrefix = "seg-"
	segmentSuffix = ".m4s"
	// segmentDigits is the zero-padded width of a segment number (seg-%05d.m4s).
	segmentDigits = 5
)

// Content types and caching of the HLS files (ADR 0024). The playlist grows while FFmpeg
// runs and must never be cached; init and media segments never change once written.
const (
	playlistContentType = "application/vnd.apple.mpegurl"
	initContentType     = "video/mp4"
	segmentContentType  = "video/iso.segment"
	playlistCaching     = "no-store"
	segmentCaching      = "private, max-age=3600"
)

// notReadyRetrySeconds is the Retry-After of a stream_not_ready answer. The client has
// waited readyTimeout already, so a short pause before the next attempt is enough.
const notReadyRetrySeconds = 2

// serveHLS serves a file of an HLS session (GET /api/v1/playback/sessions/{sessionId}/hls/
// {fileName}). Only the three name shapes of ADR 0024 are served; every other name answers
// 404 before the session is even looked up, so it can neither reach the file system nor keep
// a session alive. Sessions of another delivery answer 404.
func (handlers *Handlers) serveHLS(writer http.ResponseWriter, request *http.Request) {
	response := statuswriter.New(writer)

	id, valid := sessionID(request)
	if !valid {
		writeProblem(response, badRequestProblem())

		return
	}

	name := request.PathValue(fileNameParameter)

	description, known := hlsFileDescription(name)
	if !known {
		writeProblem(response, notFoundProblem())

		return
	}

	granted, ok := handlers.grantAccess(response, request, id)
	if !ok {
		return
	}

	defer granted.release()

	if granted.delivery != playback.DeliveryHLS {
		writeProblem(response, notFoundProblem())

		return
	}

	if !handlers.awaitStream(response, request, granted) {
		return
	}

	handlers.serveOutputFile(response, request, outputFile{sessionID: id, name: name, served: description})
}

// hlsFileDescription returns how the HLS file name is served, and false for every name that
// is not index.m3u8, init.mp4 or seg-NNNNN.m4s (exact case, five ASCII digits).
func hlsFileDescription(name string) (content, bool) {
	switch {
	case name == playlistName:
		return content{contentType: playlistContentType, cacheControl: playlistCaching}, true
	case name == initName:
		return content{contentType: initContentType, cacheControl: segmentCaching, lastModified: true}, true
	case isSegmentName(name):
		return content{contentType: segmentContentType, cacheControl: segmentCaching, lastModified: true}, true
	default:
		return content{}, false
	}
}

func isSegmentName(name string) bool {
	number, hasPrefix := strings.CutPrefix(name, segmentPrefix)
	number, hasSuffix := strings.CutSuffix(number, segmentSuffix)

	return hasPrefix && hasSuffix && len(number) == segmentDigits &&
		strings.IndexFunc(number, isNotASCIIDigit) < 0
}

func isNotASCIIDigit(character rune) bool {
	return character < '0' || character > '9'
}

// awaitStream waits up to readyTimeout, and no longer than the request, for the session's
// first segment. It answers the request itself and reports false when the stream cannot be
// served: 503 stream_not_ready with Retry-After while it is still starting, 404 when the
// production was stopped meanwhile, 500 when it failed. Every HLS file waits, not only the
// playlist: once the first segment exists the wait returns at once, and a failed production
// answers 500 for all files of the session (M2-20).
func (handlers *Handlers) awaitStream(response *statuswriter.Recorder, request *http.Request, granted grant) bool {
	waitContext, cancel := context.WithTimeout(request.Context(), handlers.readyTimeout)
	defer cancel()

	err := granted.waitReady(waitContext)

	switch {
	case err == nil:
		return true
	case waitContext.Err() != nil && errors.Is(err, waitContext.Err()):
		response.Header().Set(headerRetryAfter, strconv.Itoa(notReadyRetrySeconds))
		writeProblem(response, restapi.NewProblem(http.StatusServiceUnavailable, restapi.CodeStreamNotReady))
	case errors.Is(err, domain.ErrNotFound):
		// The session was stopped after access was granted (appplayback.ErrProductionStopped):
		// for its clients it is gone, as if Access had not found it.
		writeProblem(response, notFoundProblem())
	default:
		// The producer has logged the failure once at warn with FFmpeg's stderr tail, and the
		// access log records this 500 at info. The record here only ties the request to the
		// session; repeating the cause would log it again for every player retry.
		handlers.logger.DebugContext(request.Context(), "stream refused, its production failed",
			logtext.RequestAttributes(request, slog.String(sessionAttribute, granted.sessionID))...)
		writeProblem(response, restapi.InternalProblem())
	}

	return false
}

// outputFile is one validated file name of a session's HLS output and how it is served.
type outputFile struct {
	sessionID string
	name      string
	served    content
}

// serveOutputFile opens the file through os.Root of the session directory and serves it. A
// file FFmpeg has not written (yet) answers 404; the playlist only lists finished segments.
// Init and media segments are streamed from their files, which FFmpeg never rewrites once
// listed; the playlist, which it replaces while it runs, is served from memory.
func (handlers *Handlers) serveOutputFile(response *statuswriter.Recorder, request *http.Request, output outputFile) {
	file, err := handlers.output.OpenFile(output.sessionID, output.name)
	if errors.Is(err, domain.ErrNotFound) {
		writeProblem(response, notFoundProblem())

		return
	}

	if err != nil {
		handlers.logger.ErrorContext(request.Context(), "open stream output file",
			logtext.RequestAttributes(request, slog.String(sessionAttribute, output.sessionID), logtext.Error(err))...)
		writeProblem(response, restapi.InternalProblem())

		return
	}

	served := output.served
	served.file = file

	if output.name == playlistName {
		handlers.servePlaylist(response, request, output.sessionID, served)

		return
	}

	defer closeFile(file)

	handlers.serveContent(response, request, output.sessionID, served)
}

// servePlaylist reads the opened playlist into memory, closing it before the first response
// byte, and serves it with the start tag of withStartAtZero (ADR 0024 note M2-37). A
// playlist beyond maxPlaylistBytes is a server fault: 500, logged at error.
func (handlers *Handlers) servePlaylist(response *statuswriter.Recorder, request *http.Request, sessionID string, served content) {
	body, err := loadPlaylist(served.file)
	if errors.Is(err, domain.ErrNotFound) {
		writeProblem(response, notFoundProblem())

		return
	}

	if err != nil {
		handlers.logger.ErrorContext(request.Context(), "load stream playlist",
			logtext.RequestAttributes(request, slog.String(sessionAttribute, sessionID), logtext.Error(err))...)
		writeProblem(response, restapi.InternalProblem())

		return
	}

	handlers.send(response, request, sessionID, payload{
		body: bytes.NewReader(withStartAtZero(body)), contentType: served.contentType, cacheControl: served.cacheControl,
	})
}
