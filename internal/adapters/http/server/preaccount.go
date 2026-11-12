package server

import (
	"net/http"

	appaccess "github.com/magpeek/mivo/internal/app/access"
	domainaccess "github.com/magpeek/mivo/internal/domain/access"
)

// preAccountViewer is the viewer of the pre-account mode (Options.Authentication unset):
// an administrator without library or content limit and without a sign-in session. The
// server is open by design in that mode, exactly as before accounts existed (ADR 0032 §7,
// "Pre-account mode"), and the catalog use cases, which fail closed without a viewer, keep
// working. It is never used once Authentication is wired: a request without a valid session
// then carries no viewer at all.
//
// TODO(M7-25): remove with the pre-account mode.
func preAccountViewer() domainaccess.Viewer {
	return domainaccess.Viewer{Admin: true, Libraries: domainaccess.AllLibraries(), Limit: domainaccess.NoLimit()}
}

// withPreAccountViewer stores preAccountViewer in the context of every request.
//
// TODO(M7-25): remove with the pre-account mode.
func withPreAccountViewer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		next.ServeHTTP(writer, request.WithContext(appaccess.WithViewer(request.Context(), preAccountViewer())))
	})
}
