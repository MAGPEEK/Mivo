package stream

import (
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/logtext"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/adapters/http/statuswriter"
)

// Response header names the handlers set.
const (
	headerContentType   = "Content-Type"
	headerCacheControl  = "Cache-Control"
	headerContentLength = "Content-Length"
	headerRetryAfter    = "Retry-After"
)

// content is one opened file and how to describe it.
type content struct {
	file         *os.File
	contentType  string
	cacheControl string
	// lastModified sends the file's modification time and answers conditional requests
	// with it. The playlist is rewritten under the same name, so it goes without.
	lastModified bool
}

// serveContent sends file with http.ServeContent: Range, If-Range, HEAD, 206 and 416. The
// response writer chain keeps io.ReaderFrom, so net/http can use sendfile. No ETag is sent.
func (handlers *Handlers) serveContent(response *statuswriter.Recorder, request *http.Request, sessionID string, served content) {
	info, err := served.file.Stat()
	if err != nil {
		handlers.logger.ErrorContext(request.Context(), "inspect stream file",
			logtext.RequestAttributes(request, slog.String(sessionAttribute, sessionID), logtext.Error(err))...)
		writeProblem(response, restapi.InternalProblem())

		return
	}

	if !info.Mode().IsRegular() {
		writeProblem(response, notFoundProblem())

		return
	}

	var modified time.Time
	if served.lastModified {
		modified = info.ModTime()
	}

	header := response.Header()
	header.Set(headerContentType, served.contentType)

	if served.cacheControl != "" {
		header.Set(headerCacheControl, served.cacheControl)
	}

	http.ServeContent(response, request, "", modified, served.file)
	handlers.reportIncomplete(response, request, sessionID)
}

// reportIncomplete logs a body that ended before its declared length while the client was
// still there, for example a library disk that dropped mid-file. http.ServeContent swallows
// the copy error and the access log would record such a response as a successful 200 or
// 206 at debug level on these quiet routes (M2-08). A client that went away (a player
// seeking) cancels the request context and is not a failure.
func (handlers *Handlers) reportIncomplete(response *statuswriter.Recorder, request *http.Request, sessionID string) {
	if request.Method == http.MethodHead || request.Context().Err() != nil {
		return
	}

	if status := response.StatusOrOK(); status != http.StatusOK && status != http.StatusPartialContent {
		return
	}

	declared, err := strconv.ParseInt(response.Header().Get(headerContentLength), 10, 64)
	if err != nil || response.Written() >= declared {
		return
	}

	handlers.logger.WarnContext(request.Context(), "stream response ended before its declared length",
		logtext.RequestAttributes(request,
			slog.String(sessionAttribute, sessionID),
			slog.Int64("written", response.Written()),
			slog.Int64("declared", declared))...)
}

// writeProblem sends problem unless the status line is already out: a problem after the
// body has started would corrupt the response.
func writeProblem(response *statuswriter.Recorder, problem apigen.Problem) {
	if response.HeadersSent() {
		return
	}

	restapi.WriteProblem(response, problem)
}

func notFoundProblem() apigen.Problem {
	return restapi.NewProblem(http.StatusNotFound, restapi.CodeNotFound)
}

func badRequestProblem() apigen.Problem {
	return restapi.NewProblem(http.StatusBadRequest, restapi.CodeBadRequest)
}

// closeFile closes a file that was only read; a close error cannot lose data.
func closeFile(file *os.File) {
	_ = file.Close()
}
