package stream

import (
	"net/http"
	"net/url"

	"github.com/magpeek/mivo/internal/domain/playback"
)

// Path pieces of the stream routes (api/openapi.yaml, tag stream). URLBuilder and the mount
// patterns are built from the same constants, so a URL handed to a client always reaches its
// handler.
const (
	sessionsPath       = "/api/v1/playback/sessions/"
	sessionIDParameter = "sessionId"
	fileNameParameter  = "fileName"
	fileSuffix         = "/file"
	hlsSegment         = "/hls/"

	// filePattern serves the original file of a direct-play session; GET also matches HEAD.
	filePattern = http.MethodGet + " " + sessionsPath + "{" + sessionIDParameter + "}" + fileSuffix
	// hlsPattern serves the playlist and segments of a remux or transcode session.
	hlsPattern = http.MethodGet + " " + sessionsPath + "{" + sessionIDParameter + "}" + hlsSegment +
		"{" + fileNameParameter + "}"
)

// URLBuilder builds the stream URLs of sessions. It implements app/playback.StreamURLBuilder
// and is wired by cmd. The URLs are root-relative references (ADR 0024, Notes M2-01).
type URLBuilder struct{}

// NewURLBuilder returns a URLBuilder.
func NewURLBuilder() URLBuilder {
	return URLBuilder{}
}

// StreamURL returns the file URL of a direct-play session (playback.DeliveryFile) or the
// playlist URL of an HLS session (playback.DeliveryHLS), and "" for an unknown delivery,
// which no plan of playback.Decide has.
func (URLBuilder) StreamURL(sessionID string, delivery playback.Delivery) string {
	session := sessionsPath + url.PathEscape(sessionID)

	switch delivery {
	case playback.DeliveryFile:
		return session + fileSuffix
	case playback.DeliveryHLS:
		return session + hlsSegment + playlistName
	default:
		return ""
	}
}

// QuietPatterns returns the patterns of the stream routes for server.Options.QuietPatterns:
// segment polling is logged at debug level while it succeeds (ADR 0024).
func QuietPatterns() []string {
	return []string{filePattern, hlsPattern}
}
