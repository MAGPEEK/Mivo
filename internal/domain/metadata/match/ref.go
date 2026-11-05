package match

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

// RefProvider names the database a ProviderRef points into. MusicBrainz is listed here, not
// in metadata.Provider, because only matching and the identify dialog need it.
type RefProvider string

// Providers a pasted ID or URL can name.
const (
	RefTMDB        RefProvider = "tmdb"
	RefIMDb        RefProvider = "imdb"
	RefMusicBrainz RefProvider = "musicbrainz"
)

// RefKind says what the ID identifies where the provider has several ID spaces (TMDB movie 603
// and TMDB show 603 differ). It is empty when unknown.
type RefKind string

// Reference kinds.
const (
	RefKindUnknown RefKind = ""
	RefKindMovie   RefKind = "movie"
	RefKindShow    RefKind = "tv"
	RefKindRelease RefKind = "release"
	RefKindArtist  RefKind = "artist"
)

// ProviderRef identifies one entry in one provider's database.
type ProviderRef struct {
	Provider RefProvider
	Kind     RefKind
	ID       string
}

// IsZero reports whether the reference is unset.
func (ref ProviderRef) IsZero() bool {
	return ref.Provider == "" && ref.ID == ""
}

// ProviderID converts a TMDB or IMDb reference to the ID stored in a metadata record. It
// reports false for other providers.
func (ref ProviderRef) ProviderID() (metadata.ProviderID, bool) {
	switch ref.Provider {
	case RefTMDB:
		return metadata.ProviderID{Provider: metadata.ProviderTMDB, Value: ref.ID}, true
	case RefIMDb:
		return metadata.ProviderID{Provider: metadata.ProviderIMDb, Value: ref.ID}, true
	case RefMusicBrainz:
		return metadata.ProviderID{}, false // not a metadata.Provider; see RefProvider
	}

	return metadata.ProviderID{}, false
}

// Layout of a UUID text: 8-4-4-4-12 hexadecimal digits, the dashes at these offsets.
const (
	musicBrainzUUIDLength = 36
	uuidDashFirst         = 8
	uuidDashSecond        = 13
	uuidDashThird         = 18
	uuidDashFourth        = 23
)

// ParseProviderRef extracts provider and ID from text the user pasted: a bare TMDB number, an
// IMDb "tt…" ID, a MusicBrainz UUID, or a TMDB movie/tv, IMDb title or MusicBrainz
// release/artist URL. Anything else, including other hosts, is rejected with an error that
// wraps domain.ErrInvalid.
func ParseProviderRef(text string) (ProviderRef, error) {
	trimmed := strings.TrimSpace(text)

	if ref, ok := parseBareRef(trimmed); ok {
		return ref, nil
	}

	ref, err := parseURLRef(trimmed)
	if err != nil {
		return ProviderRef{}, fmt.Errorf("parse provider reference %q: %w", text, err)
	}

	return ref, nil
}

func parseBareRef(text string) (ProviderRef, bool) {
	switch {
	case metadata.ValidateProviderID(metadata.ProviderIMDb, text) == nil:
		return ProviderRef{Provider: RefIMDb, ID: text}, true
	case metadata.ValidateProviderID(metadata.ProviderTMDB, text) == nil:
		return ProviderRef{Provider: RefTMDB, ID: text}, true
	case isUUID(text):
		return ProviderRef{Provider: RefMusicBrainz, ID: strings.ToLower(text)}, true
	default:
		return ProviderRef{}, false
	}
}

func parseURLRef(text string) (ProviderRef, error) {
	if !strings.Contains(text, "://") {
		text = "https://" + text
	}

	parsed, err := url.Parse(text)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ProviderRef{}, fmt.Errorf("not a web address: %w", domain.ErrInvalid)
	}

	host := strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www."), "m.")
	segments := strings.FieldsFunc(parsed.Path, func(char rune) bool { return char == '/' })

	var ref ProviderRef

	switch host {
	case "themoviedb.org":
		ref = tmdbURLRef(segments)
	case "imdb.com":
		ref = imdbURLRef(segments)
	case "musicbrainz.org":
		ref = musicBrainzURLRef(segments)
	}

	if ref.IsZero() {
		return ProviderRef{}, fmt.Errorf("unsupported address: %w", domain.ErrInvalid)
	}

	return ref, nil
}

// tmdbURLRef reads /movie/603-the-matrix and /tv/1396-breaking-bad; the slug is ignored.
func tmdbURLRef(segments []string) ProviderRef {
	if len(segments) < 2 || (segments[0] != "movie" && segments[0] != "tv") {
		return ProviderRef{}
	}

	id, _, _ := strings.Cut(segments[1], "-")
	if metadata.ValidateProviderID(metadata.ProviderTMDB, id) != nil {
		return ProviderRef{}
	}

	return ProviderRef{Provider: RefTMDB, Kind: RefKind(segments[0]), ID: id}
}

func imdbURLRef(segments []string) ProviderRef {
	if len(segments) < 2 || segments[0] != "title" || metadata.ValidateProviderID(metadata.ProviderIMDb, segments[1]) != nil {
		return ProviderRef{}
	}

	return ProviderRef{Provider: RefIMDb, ID: segments[1]}
}

func musicBrainzURLRef(segments []string) ProviderRef {
	if len(segments) < 2 || (segments[0] != "release" && segments[0] != "artist") || !isUUID(segments[1]) {
		return ProviderRef{}
	}

	return ProviderRef{Provider: RefMusicBrainz, Kind: RefKind(segments[0]), ID: strings.ToLower(segments[1])}
}

// isUUID reports whether text has the 8-4-4-4-12 hexadecimal form of a MusicBrainz ID.
func isUUID(text string) bool {
	if len(text) != musicBrainzUUIDLength {
		return false
	}

	for index, character := range text {
		switch index {
		case uuidDashFirst, uuidDashSecond, uuidDashThird, uuidDashFourth:
			if character != '-' {
				return false
			}
		default:
			if !isHexDigit(character) {
				return false
			}
		}
	}

	return true
}

func isHexDigit(character rune) bool {
	return (character >= '0' && character <= '9') || (character >= 'a' && character <= 'f') || (character >= 'A' && character <= 'F')
}
