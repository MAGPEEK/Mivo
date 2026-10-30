package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/logtext"
)

// BodyReadTimeout bounds how long a client may take to send a request body, so a client
// that announces or trickles a body cannot hold a connection open. The server sets no
// ReadTimeout (see New), and ReadHeaderTimeout ends with the headers.
const BodyReadTimeout = 30 * time.Second

// limitBodyReadTime sets the connection's read deadline for every request with a body,
// before any handler runs. That also bounds what net/http reads after a handler that
// answered without reading the body (an early 413, a 405, a 404): it discards up to
// 256 KiB of the rest before it sends the answer, which would otherwise wait for a silent
// client forever.
//
// Requests without a body (http.NoBody), such as stream GETs, never get a deadline. For them
// net/http already reads the connection in the background, to notice a client that goes
// away, and a deadline hit there would cancel the request context of the long response.
// For a request with a body that background read starts when the body hits its end, and
// net/http clears the read deadline at that moment, so a handler that works on after
// reading its body keeps its context. HTTP/2 is unaffected: there the deadline applies to
// the stream's body only and no background read exists (the server speaks HTTP/1.1 today).
func limitBodyReadTime(next http.Handler, logger *slog.Logger, timeout time.Duration) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Body != nil && request.Body != http.NoBody {
			setBodyReadDeadline(writer, request, logger, timeout)
		}

		next.ServeHTTP(writer, request)
	})
}

func setBodyReadDeadline(writer http.ResponseWriter, request *http.Request, logger *slog.Logger, timeout time.Duration) {
	err := http.NewResponseController(writer).SetReadDeadline(time.Now().Add(timeout))
	if err != nil {
		// The writer of the real server supports deadlines; only a test recorder or a
		// wrapper without Unwrap lacks it. The body stays size-limited by the API.
		logger.DebugContext(request.Context(), "request body read deadline not set",
			logtext.RequestAttributes(request, slog.Any("error", err))...)
	}
}
