package playback_test

import (
	"context"
	"testing"

	appaccess "github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/domain/access"
)

// adminContext carries an unrestricted administrator, the viewer of most tests.
func adminContext(t *testing.T) context.Context {
	t.Helper()

	return viewerContext(t, access.Viewer{
		AuthSessionID: "auth-admin", AccountID: "account-admin", ProfileID: "profile-admin", Admin: true,
		Libraries: access.AllLibraries(), Limit: access.NoLimit(),
	})
}

// userContext carries a non-admin viewer of the given auth session and libraries.
func userContext(t *testing.T, authSessionID string, libraryIDs ...string) context.Context {
	t.Helper()

	return viewerContext(t, access.Viewer{
		AuthSessionID: authSessionID, AccountID: "account-" + authSessionID, ProfileID: "profile-" + authSessionID,
		Libraries: access.OnlyLibraries(libraryIDs...), Limit: access.NoLimit(),
	})
}

func viewerContext(t *testing.T, viewer access.Viewer) context.Context {
	t.Helper()

	return appaccess.WithViewer(t.Context(), viewer)
}
