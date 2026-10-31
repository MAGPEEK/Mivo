package stream

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/magpeek/mivo/internal/domain"
)

// maxPlaylistBytes bounds the in-memory copy of index.m3u8 (ADR 0024 note M2-37). The
// seg-NNNNN pattern allows at most 100,000 segments of about 32 bytes each
// ("#EXTINF:6.000000," plus "seg-NNNNN.m4s"), so 4 MiB never cuts a real playlist; a larger
// file is a server fault.
const maxPlaylistBytes = 4 << 20

// Buffer growth of readBounded when the file turns out larger than its size said: at least
// minimumGrowth bytes, else growthFactor times the buffer, so a growing file costs few copies.
const (
	minimumGrowth = 512
	growthFactor  = 2
)

// Playlist lines of RFC 8216 the served playlist is checked and amended with.
const (
	playlistHeader = "#EXTM3U"
	// startTag makes native HLS players (Safari, webOS) start a growing EVENT playlist at
	// its beginning instead of near the newest segment (RFC 8216 4.3.5.2).
	startTag               = "#EXT-X-START:TIME-OFFSET=0"
	startTagPrefix         = "#EXT-X-START:"
	lineFeed               = "\n"
	carriageReturnLineFeed = "\r\n"
)

// errPlaylistTooLarge reports a playlist beyond maxPlaylistBytes.
var errPlaylistTooLarge = errors.New("playlist exceeds its size bound")

// loadPlaylist reads the opened playlist into memory and closes it before it returns, so the
// response that follows never holds the file open (ADR 0024 note M2-37): a stalled client
// can then neither make FFmpeg's rename over it fail on Windows nor delay the final
// playlist. A file that is not regular (a directory of that name) wraps domain.ErrNotFound.
func loadPlaylist(file *os.File) ([]byte, error) {
	defer closeFile(file)

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect playlist: %w", err)
	}

	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("playlist is not a regular file: %w", domain.ErrNotFound)
	}

	body, err := readBounded(file, info.Size(), maxPlaylistBytes)
	if err != nil {
		return nil, fmt.Errorf("read playlist: %w", err)
	}

	return body, nil
}

// readBounded reads reader to its end, at most limit bytes; more wraps errPlaylistTooLarge.
// sizeHint (the file size) sizes the buffer, and growth is capped, so the buffer never
// exceeds limit plus the one byte that detects an oversized file. io.ReadAll could not
// promise that: its append growth overshoots.
func readBounded(reader io.Reader, sizeHint, limit int64) ([]byte, error) {
	bound := limit + 1
	limited := io.LimitReader(reader, bound)
	// One byte beyond the expected size leaves room for the read that reports the end.
	body := make([]byte, 0, min(max(sizeHint, 0)+1, bound))

	for int64(len(body)) < bound {
		if len(body) == cap(body) {
			body = grown(body, bound)
		}

		read, err := limited.Read(body[len(body):cap(body)])
		body = body[:len(body)+read]

		if errors.Is(err, io.EOF) {
			return body, nil
		}

		if err != nil {
			return nil, fmt.Errorf("read: %w", err)
		}
	}

	return nil, fmt.Errorf("%w of %d bytes", errPlaylistTooLarge, limit)
}

// grown returns body in a buffer growthFactor times as large, never larger than bound.
func grown(body []byte, bound int64) []byte {
	capacity := min(int64(max(growthFactor*cap(body), minimumGrowth)), bound)
	enlarged := make([]byte, len(body), capacity)
	copy(enlarged, body)

	return enlarged
}

// withStartAtZero returns playlist with startTag as its second line, directly after the
// leading #EXTM3U line, keeping that line's ending (LF or CRLF). A playlist that already has
// an EXT-X-START tag, or that does not begin with exactly #EXTM3U, is returned unchanged.
// playlist itself is never modified.
func withStartAtZero(playlist []byte) []byte {
	afterHeader, isPlaylist := bytes.CutPrefix(playlist, []byte(playlistHeader))
	if !isPlaylist {
		return playlist
	}

	lineEnd, exactHeader := headerLineEnd(afterHeader)
	if !exactHeader || hasStartTag(afterHeader) {
		return playlist
	}

	served := make([]byte, 0, len(playlist)+len(lineEnd)+len(startTag))
	served = append(served, playlistHeader...)
	served = append(served, lineEnd...)
	served = append(served, startTag...)

	return append(served, afterHeader...)
}

// headerLineEnd returns the line ending that follows #EXTM3U, and false when other
// characters follow it on the same line. A playlist of only #EXTM3U gets a line feed, so
// the start tag becomes its own (last) line.
func headerLineEnd(afterHeader []byte) (string, bool) {
	switch {
	case len(afterHeader) == 0, bytes.HasPrefix(afterHeader, []byte(lineFeed)):
		return lineFeed, true
	case bytes.HasPrefix(afterHeader, []byte(carriageReturnLineFeed)):
		return carriageReturnLineFeed, true
	default:
		return "", false
	}
}

// hasStartTag reports whether a line of playlist starts with #EXT-X-START:; RFC 8216 allows
// the tag at most once.
func hasStartTag(playlist []byte) bool {
	for line := range bytes.Lines(playlist) {
		if bytes.HasPrefix(line, []byte(startTagPrefix)) {
			return true
		}
	}

	return false
}
