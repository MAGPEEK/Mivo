// Package requestid assigns every HTTP request an ID that ties its log lines together and
// that clients can quote in bug reports. It is shared by the server middleware chain and the
// API handlers, which log with the ID but must not import the server package.
package requestid

import (
	"context"
	"net/http"
	"uuid"
)

// HeaderName is the request and response header that carries the ID.
const HeaderName = "X-Request-ID"

// maxLength bounds an ID taken over from a client, so a hostile header cannot bloat every
// log line of the request. A UUID needs 36 characters; trace IDs of common proxies fit too.
const maxLength = 128

type contextKey struct{}

// Middleware reuses a well-formed X-Request-ID from the client (a reverse proxy may have set
// it already) and generates one otherwise. The ID is echoed in the response header and
// stored in the request context for FromContext.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		id := request.Header.Get(HeaderName)
		if !isAcceptable(id) {
			id = uuid.NewV4().String()
		}

		writer.Header().Set(HeaderName, id)
		next.ServeHTTP(writer, request.WithContext(NewContext(request.Context(), id)))
	})
}

// NewContext returns a copy of ctx that carries id.
func NewContext(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

// FromContext returns the request ID stored by Middleware, or "" when there is none.
func FromContext(ctx context.Context) string {
	id, _ := ctx.Value(contextKey{}).(string)

	return id
}

// isAcceptable allows only short IDs of ASCII letters, digits and ". _ : -". Anything else
// is replaced, because the ID is echoed to clients and written to every log line.
func isAcceptable(id string) bool {
	if id == "" || len(id) > maxLength {
		return false
	}

	for index := range len(id) {
		if !isIDCharacter(id[index]) {
			return false
		}
	}

	return true
}

func isIDCharacter(character byte) bool {
	switch {
	case 'a' <= character && character <= 'z', 'A' <= character && character <= 'Z':
		return true
	case '0' <= character && character <= '9':
		return true
	default:
		return character == '-' || character == '_' || character == '.' || character == ':'
	}
}
