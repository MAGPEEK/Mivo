package server

import "unicode/utf8"

// maxLoggedValueBytes bounds client-controlled text (path, User-Agent) in log records. Header
// lines may be up to http.DefaultMaxHeaderBytes (1 MiB), and one hostile request must not
// write a megabyte into every log record about it.
const maxLoggedValueBytes = 256

// truncationMarker ends a value cut by truncateForLog, so a reader sees the value is partial.
const truncationMarker = "..."

// truncateForLog returns value unchanged when it fits into maxLoggedValueBytes and otherwise
// cuts it, marker included, to at most that many bytes. It never splits a UTF-8 sequence.
func truncateForLog(value string) string {
	if len(value) <= maxLoggedValueBytes {
		return value
	}

	cut := maxLoggedValueBytes - len(truncationMarker)
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}

	return value[:cut] + truncationMarker
}
