package artwork

import (
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
)

// Kind is the role a picture plays for an item. "square" follows with music (M3-13).
type Kind string

// Known artwork kinds. The text values are stored in the database and used in the API.
const (
	KindPoster Kind = "poster"
	// KindBackdrop is a wide background picture (Kodi "fanart").
	KindBackdrop Kind = "backdrop"
	// KindThumb is a 16:9 episode still or a landscape picture of a movie.
	KindThumb  Kind = "thumb"
	KindLogo   Kind = "logo"
	KindBanner Kind = "banner"
)

// Kinds returns every artwork kind in a stable order.
func Kinds() []Kind {
	return []Kind{KindPoster, KindBackdrop, KindThumb, KindLogo, KindBanner}
}

// ParseKind accepts the exact text of a known kind. The error wraps domain.ErrInvalid.
func ParseKind(text string) (Kind, error) {
	kind := Kind(text)
	if !kind.IsValid() {
		return "", fmt.Errorf("parse artwork kind %q: unknown kind: %w", text, domain.ErrInvalid)
	}

	return kind, nil
}

// IsValid reports whether the kind is one of the known kinds.
func (kind Kind) IsValid() bool {
	switch kind {
	case KindPoster, KindBackdrop, KindThumb, KindLogo, KindBanner:
		return true
	default:
		return false
	}
}

// String returns the text value of the kind.
func (kind Kind) String() string {
	return string(kind)
}

// SourceKind says where a picture lives. Local and embedded pictures are inside a library;
// provider downloads and user uploads (milestone 6) are files in the data directory, never
// in a library.
type SourceKind string

// Known artwork sources. The text values are stored in the database.
const (
	// SourceLocal is an image file next to the media (poster.jpg, <base>-thumb.png).
	SourceLocal SourceKind = "local"
	// SourceEmbedded is an attached-picture stream of a media file (MP4 covr, Matroska cover).
	SourceEmbedded SourceKind = "embedded"
	// SourceProvider is a picture downloaded from a metadata provider (milestone 6).
	SourceProvider SourceKind = "provider"
	// SourceUpload is a picture the user uploaded in the edit dialog (milestone 6).
	SourceUpload SourceKind = "upload"
)

// ParseSourceKind accepts the exact text of a known source. The error wraps
// domain.ErrInvalid.
func ParseSourceKind(text string) (SourceKind, error) {
	source := SourceKind(text)
	if !source.IsValid() {
		return "", fmt.Errorf("parse artwork source %q: unknown source: %w", text, domain.ErrInvalid)
	}

	return source, nil
}

// IsValid reports whether the source is one of the known sources.
func (source SourceKind) IsValid() bool {
	switch source {
	case SourceLocal, SourceEmbedded, SourceProvider, SourceUpload:
		return true
	default:
		return false
	}
}

// String returns the text value of the source.
func (source SourceKind) String() string {
	return string(source)
}

// Ref is how the API points at one artwork: its ID and its current cache tag, so a client
// builds an image URL that stays cacheable until the picture changes.
type Ref struct {
	ID  string
	Tag string
}
