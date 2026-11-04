package libraryfs

import "golang.org/x/text/unicode/norm"

// normalizedPath returns the Unicode NFC form of a root-relative path, or "" when the path is
// already NFC. macOS clients write decomposed (NFD) names over SMB; the domain parses NFC text
// only, while the raw path keeps opening the file (ADR 0027 §3).
func normalizedPath(relativePath string) string {
	if norm.NFC.IsNormalString(relativePath) {
		return ""
	}

	return norm.NFC.String(relativePath)
}
