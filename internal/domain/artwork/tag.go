package artwork

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// RenderVersion is part of every cache tag. Raise it when the rendering changes visibly
// (quality, scaling filter, format), so every client fetches the new pictures and the next
// purge removes the old cache files.
const RenderVersion = 1

// TagLength is the number of lowercase hex digits of a tag. 64 bits make an accidental
// collision between two versions of one picture practically impossible.
const TagLength = 16

// tagSeparator separates the identity parts in the hashed text. NUL never occurs in a
// relative path (library.ParseRelativePath rejects it), so two different identities never
// produce the same text.
const tagSeparator = "\x00"

// TagInput is the identity of a picture version: where it is read from, its file size and
// its modification time.
type TagInput struct {
	Source     SourceKind
	Location   Location
	Size       int64
	ModifiedAt time.Time
}

// Tag returns the cache tag of a picture version (ADR 0027 §8): the first TagLength hex
// digits of SHA-256 over source, location, size, modification time in Unix milliseconds
// (the precision the catalog stores) and RenderVersion. The tag changes whenever the file
// or the rendering changes, which makes long-lived immutable caching safe.
func Tag(input TagInput) string {
	parts := []string{
		input.Source.String(),
		input.Location.RelativePath,
		strconv.Itoa(input.Location.StreamIndex),
		input.Location.DataPath,
		strconv.FormatInt(input.Size, 10),
		strconv.FormatInt(input.ModifiedAt.UnixMilli(), 10),
		strconv.Itoa(RenderVersion),
	}

	digest := sha256.Sum256([]byte(strings.Join(parts, tagSeparator)))

	return hex.EncodeToString(digest[:])[:TagLength]
}
