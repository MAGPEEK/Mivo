package server

import (
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/logtext"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	appaccess "github.com/magpeek/mivo/internal/app/access"
)

// Fixed details of the management guard's 403 problem, one per guarded area, so the client
// can tell the user what is refused.
const (
	manageRefusalDetail           = "Library management needs an administrator on the local network."
	metadataSettingsRefusalDetail = "Metadata settings need an administrator on the local network."
	metadataRefusalDetail         = "Metadata management needs an administrator on the local network."
	usersRefusalDetail            = "User administration needs an administrator on the local network."
	devicesRefusalDetail          = "Device administration needs an administrator on the local network."
	setupRefusalDetail            = "The first-run setup is only available from the local network."
	profilesRefusalDetail         = "Administrators manage profiles from the local network only."
	sessionsRefusalDetail         = "The running playback sessions are only shown to an administrator on the local network."
	transcoderRefusalDetail       = "Transcoder settings need an administrator on the local network."
)

// manageArea is the guarded area a request belongs to; areaNone is not guarded.
type manageArea int

const (
	areaNone manageArea = iota
	areaLibraries
	areaMetadataSettings
	areaMetadata
	areaUsers
	areaDevices
	// areaSetup needs a local peer and the same-origin rule, but no viewer: nobody can be
	// signed in before the first account exists.
	areaSetup
	// areaProfiles refuses only admin viewers outside the local network; owners pass to the
	// use case's ownership check.
	areaProfiles
	// areaSessions is the list of all running playback sessions; starting, reading and stopping
	// one's own session stays open to every profile.
	areaSessions
	// areaTranscoder is the hardware acceleration status and its test run.
	areaTranscoder
)

// Paths of the guarded routes (api/openapi.yaml).
const (
	apiV1Libraries        = "/api/v1/libraries"
	apiV1Roots            = "/api/v1/folder-roots"
	apiV1Items            = "/api/v1/items"
	apiV1MetadataSettings = "/api/v1/settings/metadata"
	apiV1Posters          = "/api/v1/metadata/posters"
	apiV1Users            = "/api/v1/users"
	apiV1Devices          = "/api/v1/devices"
	apiV1Setup            = "/api/v1/setup"
	apiV1Profiles         = "/api/v1/profiles"
	apiV1Playback         = "/api/v1/playback"
	apiV1Transcoder       = "/api/v1/system/transcoder"

	// settingsSegments and posterSegments are the path segments of the settings route and of
	// one poster ("api", "v1", ... and the poster ID).
	settingsSegments = 4
	posterSegments   = 5
	// collectionSegments counts "api", "v1" and a collection name; userProfilesSegments adds
	// the account ID and "profiles".
	collectionSegments   = 3
	userProfilesSegments = 5
	// sessionListSegments counts "api", "v1", "playback" and "sessions".
	sessionListSegments = 4
)

// manageGuard protects the management and administration operations (ADR 0028, ADR 0032
// §9): creating, changing and deleting libraries, starting a scan, browsing the server's
// folders, the online metadata operations, the user and device administration and the
// first-run setup (see managementArea). It refuses a request with 403 forbidden when
//
//   - the client is not in the local network, also when server.lan_only is off: that
//     setting opens the media endpoints to the internet, never the administration;
//   - the request breaks the same-origin rule (crossOriginRefusal);
//   - authentication is on and the viewer is no admin (every area but the setup).
//
// The client is the resolved origin, as in lanGuard: the socket peer, or the client named by
// a reverse proxy listed in server.trusted_proxies; forwarding headers of any other peer are
// ignored, and a listed proxy that names no client counts as remote (ADR 0032 §3).
// The profiles of the caller's own account (/users/{ownId}/profiles) are not guarded, and
// /profiles/{id} only refuses admins outside the local network (see profileRefusal): the use
// case checks that the caller owns them (CanManageProfile).
//
// Residual risks (ADR 0032 §9): a reverse proxy or tunnel that connects from the LAN or the
// host itself and is not listed in server.trusted_proxies makes every client look local, so
// the admin sign-in is the remaining check;
// DNS rebinding can make a hostile page address the server under its own name, in which case
// Origin and Host match, but it cannot reach admin endpoints without the SameSite=Lax session
// cookie. The Host header is no defence (BACKLOG m3-19k keeps the Host allow-list open).
type manageGuard struct {
	logger *slog.Logger
	lan    lanGuard
	// requireAdmin is on once authentication is wired (Options.Authentication).
	requireAdmin bool
}

func newManageGuard(settings chainSettings, lan lanGuard) manageGuard {
	return manageGuard{logger: settings.logger, lan: lan, requireAdmin: settings.authentication.enabled()}
}

func (guard manageGuard) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		area := managementArea(request)
		if area == areaNone || (area == areaUsers && isOwnProfilesRequest(request)) {
			next.ServeHTTP(writer, request)

			return
		}

		if reason := guard.refusal(request, area); reason != "" {
			guard.logger.DebugContext(request.Context(), "management request refused",
				logtext.RequestAttributes(request,
					append([]any{slog.String("reason", reason)}, originAttributes(request)...)...)...)
			restapi.WriteProblem(writer, manageRefusalProblem(area))

			return
		}

		next.ServeHTTP(writer, request)
	})
}

// refusal names why the request is refused, or returns "" when it may pass.
func (guard manageGuard) refusal(request *http.Request, area manageArea) string {
	if area == areaProfiles {
		return guard.profileRefusal(request)
	}

	if !guard.lan.admitsPeer(request) {
		return "peer outside the local network"
	}

	if reason := crossOriginRefusal(request); reason != "" {
		return reason
	}

	if guard.requireAdmin && area != areaSetup && !isAdminViewer(request) {
		return "viewer is no administrator"
	}

	return ""
}

// profileRefusal guards /api/v1/profiles/{id}. Owners may manage their own managed profiles
// from anywhere, and the use case checks ownership (CanManageProfile); only an admin may
// reach other accounts' profiles, which ADR 0032 §9 allows from the local network only. The
// guard cannot tell whose profile the ID names, so it refuses every admin viewer outside the
// local network (admin from remote networks waits for M7b). Cookie writes already passed the
// same-origin rule in the authentication middleware.
func (guard manageGuard) profileRefusal(request *http.Request) string {
	if guard.requireAdmin && isAdminViewer(request) && !guard.lan.admitsPeer(request) {
		return "administrator outside the local network"
	}

	return ""
}

// isAdminViewer reports an admin viewer stored by the authentication middleware; a missing
// viewer is no admin (fail closed).
func isAdminViewer(request *http.Request) bool {
	viewer, ok := appaccess.ViewerFrom(request.Context())

	return ok && viewer.Admin
}

// isOwnProfilesRequest reports a request to /api/v1/users/{accountId}/profiles of the
// viewer's own account. Only a plain path counts, so an encoded or unclean variant stays
// guarded.
func isOwnProfilesRequest(request *http.Request) bool {
	viewer, ok := appaccess.ViewerFrom(request.Context())
	if !ok || viewer.AccountID == "" {
		return false
	}

	plain, ok := plainPath(request)
	if !ok {
		return false
	}

	segments := strings.Split(strings.Trim(plain, "/"), "/")

	return len(segments) == userProfilesSegments && isPrefixOf(segments, apiV1Users) &&
		segments[3] == viewer.AccountID && segments[4] == "profiles"
}

func manageRefusalProblem(area manageArea) apigen.Problem {
	problem := restapi.NewProblem(http.StatusForbidden, restapi.CodeForbidden)
	detail := areaRefusalDetail(area)
	problem.Detail = &detail

	return problem
}

func areaRefusalDetail(area manageArea) string {
	switch area {
	case areaMetadataSettings:
		return metadataSettingsRefusalDetail
	case areaMetadata:
		return metadataRefusalDetail
	case areaUsers:
		return usersRefusalDetail
	case areaDevices:
		return devicesRefusalDetail
	case areaSetup:
		return setupRefusalDetail
	case areaProfiles:
		return profilesRefusalDetail
	case areaSessions:
		return sessionsRefusalDetail
	case areaTranscoder:
		return transcoderRefusalDetail
	case areaNone, areaLibraries:
		return manageRefusalDetail
	default:
		return manageRefusalDetail
	}
}

// managementArea names the area of a request the guard protects, areaNone for the rest:
//
//   - libraries: every write to the library collection, to one library and to its scan
//     trigger, and every request to the folder browser, which shows the server's directory
//     tree. Reads of the libraries stay open to the normal clients.
//   - metadata settings: reading and changing the online metadata settings (ADR 0031).
//   - metadata: identify search and apply, removing a match, the item and library refreshes
//     and the candidate poster previews (M6-13). The metadata status stays open.
//   - users and devices: every request below /api/v1/users and /api/v1/devices.
//   - setup: every write to /api/v1/setup; reading the setup state stays open.
//   - profiles: every request below /api/v1/profiles (see profileRefusal).
//   - sessions: reading the list at /api/v1/playback/sessions (GET and HEAD, ADR 0030 §7).
//   - transcoder: every request below /api/v1/system/transcoder (ADR 0030 §5).
//
// The path is classified twice and the first guarded answer wins (fail closed): once split
// into segments that are unescaped one by one, as ServeMux matches them, so an encoded slash
// inside an ID (items/a%2Fb/identify) keeps the segment count; and once from the fully
// decoded path, so encoded letters or slashes in the fixed parts cannot slip past either.
func managementArea(request *http.Request) manageArea {
	isRead := request.Method == http.MethodGet || request.Method == http.MethodHead

	if area := segmentArea(routerSegments(request), isRead); area != areaNone {
		return area
	}

	return segmentArea(strings.Split(strings.Trim(request.URL.Path, "/"), "/"), isRead)
}

// routerSegments splits the escaped path at its literal slashes and unescapes each segment,
// as ServeMux does when it matches a pattern. A segment that does not unescape stays raw.
func routerSegments(request *http.Request) []string {
	segments := strings.Split(strings.Trim(request.URL.EscapedPath(), "/"), "/")
	for index, segment := range segments {
		if unescaped, err := url.PathUnescape(segment); err == nil {
			segments[index] = unescaped
		}
	}

	return segments
}

// segmentArea classifies the path segments of one reading of the request path.
func segmentArea(segments []string, isRead bool) manageArea {
	switch {
	case isPrefixOf(segments, apiV1Roots):
		// /api/v1/folder-roots and /api/v1/folder-roots/{rootId}/folders
		return areaIf(len(segments) == collectionSegments || (len(segments) == 5 && segments[4] == "folders"), areaLibraries)
	case isPrefixOf(segments, apiV1Libraries):
		return libraryArea(segments, isRead)
	case isPrefixOf(segments, apiV1Items):
		// /api/v1/items/{id}/identify, .../identify/search, .../match and .../refresh
		return areaIf(!isRead && len(segments) >= 5 && slices.Contains(itemMetadataSegments(), segments[4]), areaMetadata)
	case isPrefixOf(segments, apiV1MetadataSettings):
		return areaIf(len(segments) == settingsSegments, areaMetadataSettings)
	case isPrefixOf(segments, apiV1Posters):
		// /api/v1/metadata/posters/{posterId}
		return areaIf(len(segments) == posterSegments, areaMetadata)
	default:
		return streamingArea(segments, isRead)
	}
}

// streamingArea classifies the session list and the transcoder (M4-14b): reading the list
// of every viewer's sessions and the whole transcoder route are administration, while one
// session below it (start, read, stop, stream) and the trickplay are not.
func streamingArea(segments []string, isRead bool) manageArea {
	switch {
	case isPrefixOf(segments, apiV1Transcoder):
		return areaTranscoder
	case isPrefixOf(segments, apiV1Playback):
		// The collection only, also with a trailing slash; a POST starts the viewer's own
		// session.
		return areaIf(isRead && isSessionList(segments), areaSessions)
	default:
		return accountArea(segments, isRead)
	}
}

// isSessionList reports the collection path /api/v1/playback/sessions, tolerating empty
// trailing segments (a trailing slash).
func isSessionList(segments []string) bool {
	trimmed := segments
	for len(trimmed) > sessionListSegments && trimmed[len(trimmed)-1] == "" {
		trimmed = trimmed[:len(trimmed)-1]
	}

	return len(trimmed) == sessionListSegments && trimmed[3] == "sessions"
}

// accountArea classifies the account administration and the setup.
func accountArea(segments []string, isRead bool) manageArea {
	switch {
	case isPrefixOf(segments, apiV1Users):
		return areaUsers
	case isPrefixOf(segments, apiV1Devices):
		return areaDevices
	case isPrefixOf(segments, apiV1Profiles):
		return areaProfiles
	case isPrefixOf(segments, apiV1Setup):
		return areaIf(!isRead && len(segments) == collectionSegments, areaSetup)
	default:
		return areaNone
	}
}

// libraryArea classifies writes below /api/v1/libraries: the collection, one library and
// its scan trigger are library management, its metadata refresh is metadata management.
func libraryArea(segments []string, isRead bool) manageArea {
	switch {
	case isRead:
		return areaNone
	case len(segments) == 3 || len(segments) == 4 || (len(segments) == 5 && segments[4] == "scan"):
		return areaLibraries
	case len(segments) == 5 && segments[4] == "refresh":
		return areaMetadata
	default:
		return areaNone
	}
}

// itemMetadataSegments are the item sub-routes that change an item's online metadata.
func itemMetadataSegments() []string {
	return []string{"identify", "match", "refresh"}
}

func areaIf(condition bool, area manageArea) manageArea {
	if condition {
		return area
	}

	return areaNone
}

// isPrefixOf reports whether the leading segments spell prefix, a slash-separated path.
func isPrefixOf(segments []string, prefix string) bool {
	want := strings.Split(strings.Trim(prefix, "/"), "/")
	if len(segments) < len(want) {
		return false
	}

	for index, segment := range want {
		if segments[index] != segment {
			return false
		}
	}

	return true
}
