package main

import (
	"errors"
	"fmt"
	"strings"
)

// Tags of the HLS media playlists the server writes (ADR 0024, RFC 8216).
const (
	playlistHeader  = "#EXTM3U"
	playlistEndList = "#EXT-X-ENDLIST"
	mapTagPrefix    = "#EXT-X-MAP:"
	uriAttribute    = "URI"
)

var (
	errNotAPlaylist   = errors.New("not an HLS playlist")
	errNoInitSegment  = errors.New("the playlist has no EXT-X-MAP init segment")
	errNoMediaSegment = errors.New("the finished playlist lists no media segment")
)

// mediaPlaylist is what the smoke test needs from an fMP4 HLS media playlist.
type mediaPlaylist struct {
	// ended is true once the playlist carries EXT-X-ENDLIST: FFmpeg finished the stream.
	ended bool
	// initURI is the URI of the EXT-X-MAP init segment.
	initURI string
	// segments are the media segment URIs in playlist order.
	segments []string
}

// parsePlaylist reads a media playlist. An unfinished playlist may still lack segments; a
// finished one must have an init segment and at least one media segment.
func parsePlaylist(text string) (mediaPlaylist, error) {
	var playlist mediaPlaylist

	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if strings.TrimSpace(lines[0]) != playlistHeader {
		return mediaPlaylist{}, fmt.Errorf("%w: the first line is %q", errNotAPlaylist, lines[0])
	}

	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)

		switch {
		case line == "":
		case line == playlistEndList:
			playlist.ended = true
		case strings.HasPrefix(line, mapTagPrefix):
			playlist.initURI = attributeValue(strings.TrimPrefix(line, mapTagPrefix), uriAttribute)
		case strings.HasPrefix(line, "#"):
		default:
			playlist.segments = append(playlist.segments, line)
		}
	}

	if playlist.initURI == "" && (playlist.ended || len(playlist.segments) > 0) {
		return mediaPlaylist{}, errNoInitSegment
	}

	if playlist.ended && len(playlist.segments) == 0 {
		return mediaPlaylist{}, errNoMediaSegment
	}

	return playlist, nil
}

// attributeValue returns the value of name in an HLS attribute list such as
// `URI="init.mp4",BYTERANGE="720@0"`, without quotes; "" when it is missing. Quoted values
// may contain commas, so the list is split outside quotes only.
func attributeValue(list, name string) string {
	for _, attribute := range splitAttributes(list) {
		key, value, found := strings.Cut(attribute, "=")
		if found && strings.TrimSpace(key) == name {
			return strings.Trim(strings.TrimSpace(value), `"`)
		}
	}

	return ""
}

func splitAttributes(list string) []string {
	var (
		attributes []string
		current    strings.Builder
		quoted     bool
	)

	for _, character := range list {
		switch {
		case character == '"':
			quoted = !quoted
		case character == ',' && !quoted:
			attributes = append(attributes, current.String())
			current.Reset()

			continue
		}

		current.WriteRune(character)
	}

	return append(attributes, current.String())
}
