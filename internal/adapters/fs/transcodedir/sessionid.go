package transcodedir

import "uuid"

// isSessionID reports whether text is a UUID in canonical lowercase form
// (xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx). uuid.Parse also accepts uppercase, braces, URN and
// hyphen-less forms; requiring the round trip to be identical leaves exactly one spelling per
// session, so a directory name maps back to one session and nothing else is ever deleted.
func isSessionID(text string) bool {
	parsed, err := uuid.Parse(text)

	return err == nil && parsed.String() == text
}
