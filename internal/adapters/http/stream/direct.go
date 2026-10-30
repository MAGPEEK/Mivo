package stream

import (
	"log/slog"
	"net/http"

	"github.com/magpeek/mivo/internal/adapters/http/logtext"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/adapters/http/statuswriter"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// serveFile serves the original file of a direct-play session (GET and HEAD
// /api/v1/playback/sessions/{sessionId}/file). The file is the one the session grants; the
// request only names the session. Sessions of another delivery answer 404.
func (handlers *Handlers) serveFile(writer http.ResponseWriter, request *http.Request) {
	response := statuswriter.New(writer)

	id, valid := sessionID(request)
	if !valid {
		writeProblem(response, badRequestProblem())

		return
	}

	granted, ok := handlers.grantAccess(response, request, id)
	if !ok {
		return
	}

	// Held until the last byte is out, so a long Range response keeps the session alive.
	defer granted.release()

	if granted.delivery != playback.DeliveryFile {
		writeProblem(response, notFoundProblem())

		return
	}

	file, err := handlers.library.Open(granted.libraryRoot, granted.relativePath)
	if err != nil {
		// Every failure is worth a warning here: the session was started for this file, so
		// it vanished, moved out of reach or sits on storage that dropped.
		handlers.logger.WarnContext(request.Context(), "open direct-play file",
			logtext.RequestAttributes(request, slog.String(sessionAttribute, id), logtext.Error(err))...)
		writeProblem(response, restapi.ProblemForError(err))

		return
	}

	defer closeFile(file)

	handlers.serveContent(response, request, id, content{
		file: file, contentType: granted.contentType, lastModified: true,
	})
}
