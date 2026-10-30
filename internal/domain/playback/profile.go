package playback

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

// Bounds of a client profile (ADR 0023, api/openapi.yaml). They keep a hostile or broken
// client from making every decision walk huge lists.
const (
	maxProfileNameLength  = 64
	maxDirectPlayEntries  = 16
	maxCodecsPerList      = 32
	maxCodecLimits        = 32
	maxLimitProfiles      = 16
	maxLimitProfileLength = 64
	maxIdentifierLength   = 32
	maxLimitBitDepth      = 16
	unlimitedBitDepth     = 0
)

// identifierPunctuation may appear in an identifier, but not as its first character.
const identifierPunctuation = "_.-"

const profileValidationPrefix = "validate client profile"

// HLSDelivery is how a client plays HLS with fMP4 segments (API HlsDelivery).
type HLSDelivery string

// The HLS deliveries of ADR 0023.
const (
	// HLSDeliveryMSE: a script player over Media Source Extensions, for example hls.js.
	HLSDeliveryMSE HLSDelivery = "mse"
	// HLSDeliveryNative: the media element plays HLS itself.
	HLSDeliveryNative HLSDelivery = "native"
	// HLSDeliveryNone: no HLS at all; only direct play is possible.
	HLSDeliveryNone HLSDelivery = "none"
)

func (delivery HLSDelivery) isKnown() bool {
	switch delivery {
	case HLSDeliveryMSE, HLSDeliveryNative, HLSDeliveryNone:
		return true
	default:
		return false
	}
}

// ClientProfile is what a client plays (API ClientProfile). Browsers build it in
// player-core; the TV app and DLNA renderers produce the same structure later.
type ClientProfile struct {
	// Name is free text for logs and the overlay.
	Name string
	// DirectPlay lists the containers the client plays as a progressive file. Entries for
	// the same container are combined. Keys that are no direct-play key of ADR 0023 are
	// tolerated and never match, so newer clients keep working with this server.
	DirectPlay  []DirectPlayProfile
	HLS         HLSProfile
	CodecLimits []CodecLimit
}

// DirectPlayProfile is a container the client plays as a progressive file with the codecs
// it plays inside it.
type DirectPlayProfile struct {
	Container   media.DirectPlayContainer
	VideoCodecs []string
	AudioCodecs []string
}

// HLSProfile lists the codecs the client plays inside HLS with fMP4 segments.
type HLSProfile struct {
	Delivery    HLSDelivery
	VideoCodecs []string
	AudioCodecs []string
}

// CodecLimit is an extra limit of the client for one codec; a codec without an entry has
// none. Several entries for one codec must all hold.
type CodecLimit struct {
	Codec string
	// MaxBitDepth is the highest bit depth the client decodes; 0 means unlimited.
	MaxBitDepth int
	// Profiles are the codec profiles the client decodes, as ffprobe names them; empty
	// means every profile.
	Profiles []string
}

// Validate checks the bounds of ADR 0023: the name and profile texts are valid UTF-8 within
// their length, lists stay within their size, codec and container names are lower-case
// identifiers, the HLS delivery is known and bit depths lie between 0 and 16. Errors wrap
// domain.ErrInvalid.
func (profile ClientProfile) Validate() error {
	if err := validateText("name", profile.Name, maxProfileNameLength); err != nil {
		return fmt.Errorf("%s: %w", profileValidationPrefix, err)
	}

	if err := validateCount("direct-play entries", len(profile.DirectPlay), maxDirectPlayEntries); err != nil {
		return fmt.Errorf("%s: %w", profileValidationPrefix, err)
	}

	for index, entry := range profile.DirectPlay {
		if err := entry.validate(); err != nil {
			return fmt.Errorf("%s: direct play %d: %w", profileValidationPrefix, index, err)
		}
	}

	if err := profile.HLS.validate(); err != nil {
		return fmt.Errorf("%s: hls: %w", profileValidationPrefix, err)
	}

	if err := validateCount("codec limits", len(profile.CodecLimits), maxCodecLimits); err != nil {
		return fmt.Errorf("%s: %w", profileValidationPrefix, err)
	}

	for index, limit := range profile.CodecLimits {
		if err := limit.validate(); err != nil {
			return fmt.Errorf("%s: codec limit %d: %w", profileValidationPrefix, index, err)
		}
	}

	return nil
}

func (entry DirectPlayProfile) validate() error {
	if !isIdentifier(string(entry.Container)) {
		return fmt.Errorf("container %q is no lower-case identifier: %w", entry.Container, domain.ErrInvalid)
	}

	return validateCodecLists(entry.VideoCodecs, entry.AudioCodecs)
}

func (hls HLSProfile) validate() error {
	if !hls.Delivery.isKnown() {
		return fmt.Errorf("unknown delivery %q: %w", hls.Delivery, domain.ErrInvalid)
	}

	return validateCodecLists(hls.VideoCodecs, hls.AudioCodecs)
}

func (limit CodecLimit) validate() error {
	if !isIdentifier(limit.Codec) {
		return fmt.Errorf("codec %q is no lower-case identifier: %w", limit.Codec, domain.ErrInvalid)
	}

	if limit.MaxBitDepth < unlimitedBitDepth || limit.MaxBitDepth > maxLimitBitDepth {
		return fmt.Errorf("max bit depth %d outside 0..%d: %w", limit.MaxBitDepth, maxLimitBitDepth, domain.ErrInvalid)
	}

	if err := validateCount("profiles", len(limit.Profiles), maxLimitProfiles); err != nil {
		return err
	}

	for _, profile := range limit.Profiles {
		if err := validateText("profile", profile, maxLimitProfileLength); err != nil {
			return err
		}
	}

	return nil
}

func validateCodecLists(videoCodecs, audioCodecs []string) error {
	for _, list := range []struct {
		name   string
		codecs []string
	}{{name: "video codecs", codecs: videoCodecs}, {name: "audio codecs", codecs: audioCodecs}} {
		if err := validateCount(list.name, len(list.codecs), maxCodecsPerList); err != nil {
			return err
		}

		for _, codec := range list.codecs {
			if !isIdentifier(codec) {
				return fmt.Errorf("%s: %q is no lower-case identifier: %w", list.name, codec, domain.ErrInvalid)
			}
		}
	}

	return nil
}

func validateCount(name string, count, limit int) error {
	if count > limit {
		return fmt.Errorf("%d %s, at most %d allowed: %w", count, name, limit, domain.ErrInvalid)
	}

	return nil
}

// validateText bounds free text in runes, as the API's maxLength counts characters, and
// rejects control characters (CR, LF, ESC, DEL, C1, ...): the text reaches logs and the
// overlay, where they could forge lines or terminal output.
func validateText(name, text string, maxLength int) error {
	if !utf8.ValidString(text) {
		return fmt.Errorf("%s is not valid UTF-8: %w", name, domain.ErrInvalid)
	}

	if strings.ContainsFunc(text, unicode.IsControl) {
		return fmt.Errorf("%s contains a control character: %w", name, domain.ErrInvalid)
	}

	if length := utf8.RuneCountInString(text); length > maxLength {
		return fmt.Errorf("%s has %d characters, at most %d allowed: %w", name, length, maxLength, domain.ErrInvalid)
	}

	return nil
}

// isIdentifier matches the API's MediaIdentifier pattern ^[a-z0-9][a-z0-9_.-]{0,31}$
// without a package-level regular expression.
func isIdentifier(text string) bool {
	if text == "" || len(text) > maxIdentifierLength {
		return false
	}

	for index := range len(text) {
		char := text[index]
		lowerOrDigit := (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9')

		if !lowerOrDigit && (index == 0 || strings.IndexByte(identifierPunctuation, char) < 0) {
			return false
		}
	}

	return true
}
