package musicbrainz

import "strings"

// Layout of an MBID: a UUID of 8-4-4-4-12 hexadecimal digits with dashes at these offsets.
const (
	mbidLength = 36
	dashFirst  = 8
	dashSecond = 13
	dashThird  = 18
	dashFourth = 23
)

// normalizeMBID lower-cases id and reports whether it is a well-formed MBID. Only checked
// IDs are put into a request path, so a reference can never inject path segments.
func normalizeMBID(id string) (string, bool) {
	lowered := strings.ToLower(strings.TrimSpace(id))
	if len(lowered) != mbidLength {
		return "", false
	}

	for index := range len(lowered) {
		char := lowered[index]

		switch index {
		case dashFirst, dashSecond, dashThird, dashFourth:
			if char != '-' {
				return "", false
			}
		default:
			if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
				return "", false
			}
		}
	}

	return lowered, true
}
