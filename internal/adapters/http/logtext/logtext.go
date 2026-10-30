// Package logtext bounds client-controlled text before it goes into a log record. The server
// middleware, the REST API and the web UI all log request methods, paths, headers and parser
// errors that quote client input, and none of them may import another for it.
package logtext

import (
	"log/slog"
	"net/http"
	"unicode/utf8"

	"github.com/magpeek/mivo/internal/adapters/http/requestid"
)

// MaxBytes bounds client-controlled text (method, path, User-Agent, error text quoting client
// input) in log records, marker included. Header lines may be up to
// http.DefaultMaxHeaderBytes (1 MiB), and one hostile request must not write a megabyte into
// every log record about it.
const MaxBytes = 256

// TruncationMarker marks where a value was cut, so a reader sees the value is partial.
const TruncationMarker = "..."

// headShareDivisor gives the start half of the kept bytes in TruncateMiddleTo and the end
// the rest (one byte more for an odd count, because causes sit at the end).
const headShareDivisor = 2

// Truncate returns value unchanged when it fits into MaxBytes and otherwise keeps its start,
// cut so that the result, marker included, has at most MaxBytes. It never splits a UTF-8
// sequence. Use it for values whose start identifies them, such as paths.
func Truncate(value string) string {
	if len(value) <= MaxBytes {
		return value
	}

	cut := runeStartAtOrBefore(value, MaxBytes-len(TruncationMarker))

	return value[:cut] + TruncationMarker
}

// TruncateMiddle is TruncateMiddleTo with MaxBytes. Use it for error text: wrapped errors
// put the operation first and the cause last, and a quoted client value in between.
func TruncateMiddle(value string) string {
	return TruncateMiddleTo(value, MaxBytes)
}

// TruncateMiddleTo returns value unchanged when it fits into limit bytes and otherwise keeps
// its start and its end around the marker, with at most limit bytes in total. It never
// splits a UTF-8 sequence. A limit below the marker's length yields at most limit bytes of
// the marker.
func TruncateMiddleTo(value string, limit int) string {
	if len(value) <= limit {
		return value
	}

	if limit < len(TruncationMarker) {
		return TruncationMarker[:max(limit, 0)]
	}

	kept := limit - len(TruncationMarker)
	headBytes := kept / headShareDivisor
	headEnd := runeStartAtOrBefore(value, headBytes)
	tailStart := runeStartAtOrAfter(value, len(value)-(kept-headBytes))

	return value[:headEnd] + TruncationMarker + value[tailStart:]
}

// RequestAttributes returns the attributes every HTTP log record about request starts with:
// method and path cut to MaxBytes and the request ID, followed by extra.
func RequestAttributes(request *http.Request, extra ...any) []any {
	return append([]any{
		slog.String("method", Truncate(request.Method)),
		slog.String("path", Truncate(request.URL.Path)),
		slog.String("request_id", requestid.FromContext(request.Context())),
	}, extra...)
}

// Error returns err as an "error" attribute, cut by TruncateMiddle, for errors that may
// quote client input.
func Error(err error) slog.Attr {
	return slog.String("error", TruncateMiddle(err.Error()))
}

// runeStartAtOrBefore returns the largest index at most index where a rune starts, or 0.
func runeStartAtOrBefore(value string, index int) int {
	for index > 0 && !utf8.RuneStart(value[index]) {
		index--
	}

	return index
}

// runeStartAtOrAfter returns the smallest index at least index where a rune starts, or
// len(value).
func runeStartAtOrAfter(value string, index int) int {
	for index < len(value) && !utf8.RuneStart(value[index]) {
		index++
	}

	return index
}
