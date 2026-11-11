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
	maxSubtitleFormats    = 8
)

// Bounds of the audio limits (api/openapi.yaml ClientProfile). The ranges are wide on purpose:
// they reject nonsense, not unusual hardware (32 channels, 768 kHz and 64-bit float exist).
const (
	// audioLimitOmitted is a limit the client left out; the server's default applies.
	audioLimitOmitted     = 0
	minProfileChannels    = 1
	maxProfileChannels    = 32
	minProfileSampleRate  = 8000
	maxProfileSampleRate  = 768000
	minProfileAudioDepth  = 8
	maxProfileAudioDepth  = 64
	unlimitedAudioSamples = 0
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
	// Tracks is how the profile wants audio and subtitles picked without a choice in the
	// request (M4-04a). The zero value prefers no language: audio is the non-commentary
	// track with the default disposition, else the first one, and only a forced subtitle in the
	// audio language is shown.
	Tracks TrackPreference
	// SubtitleFormats are the subtitle formats the client renders itself (API subtitleFormats,
	// today only "webvtt"). Only the bounds are checked here.
	SubtitleFormats []string
	// MaxAudioChannels is the highest channel count the client plays; 0 means the client did
	// not say and DefaultMaxAudioChannels applies. Six or more keeps 5.1 when audio is
	// re-encoded, less gets a stereo downmix (ADR 0030 section 4).
	MaxAudioChannels int
	// MaxAudioSampleRate (Hz) and MaxAudioBitDepth (bits per sample) are the limits of the
	// client as a whole, apart from CodecLimits, which describe video (ADR 0030 Notes
	// (M4-02b)). Audio above them is resampled (ReasonHiResDownsampled); 0 means no limit.
	MaxAudioSampleRate int
	MaxAudioBitDepth   int
}

// audioLimits are the audio limits of a profile with the server's defaults filled in.
type audioLimits struct {
	// channels is at least 1.
	channels int
	// sampleRate and bitDepth are unlimitedAudioSamples when the client has no limit.
	sampleRate int
	bitDepth   int
}

func (profile ClientProfile) audioLimits() audioLimits {
	limits := audioLimits{
		channels: DefaultMaxAudioChannels, sampleRate: profile.MaxAudioSampleRate, bitDepth: profile.MaxAudioBitDepth,
	}

	if profile.MaxAudioChannels != audioLimitOmitted {
		limits.channels = profile.MaxAudioChannels
	}

	return limits
}

// exceededBy reports audio above the sample-rate or bit-depth limit. An unknown value passes:
// lossy codecs have no sample depth, and a missing rate is a probe quirk.
func (limits audioLimits) exceededBy(stream media.Stream) bool {
	aboveRate := limits.sampleRate != unlimitedAudioSamples && stream.SampleRate > limits.sampleRate
	aboveDepth := limits.bitDepth != unlimitedAudioSamples && stream.BitDepth > limits.bitDepth

	return aboveRate || aboveDepth
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
// identifiers, the HLS delivery is known, bit depths lie between 0 and 16, the track
// preference holds bounded language tags and a known subtitle mode, at most eight subtitle
// formats are listed and the audio limits are 0 (omitted) or within 1..32 channels,
// 8000..768000 Hz and 8..64 bits. Errors wrap domain.ErrInvalid, which adapters answer as
// 400 "invalid".
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

	if err := profile.Tracks.Validate(); err != nil {
		return fmt.Errorf("%s: tracks: %w", profileValidationPrefix, err)
	}

	if err := profile.validateSubtitleFormats(); err != nil {
		return fmt.Errorf("%s: %w", profileValidationPrefix, err)
	}

	if err := profile.validateAudioLimits(); err != nil {
		return fmt.Errorf("%s: %w", profileValidationPrefix, err)
	}

	return nil
}

func (profile ClientProfile) validateSubtitleFormats() error {
	if err := validateCount("subtitle formats", len(profile.SubtitleFormats), maxSubtitleFormats); err != nil {
		return err
	}

	for _, format := range profile.SubtitleFormats {
		if !isIdentifier(format) {
			return fmt.Errorf("subtitle format %q is no lower-case identifier: %w", format, domain.ErrInvalid)
		}
	}

	return nil
}

// validateAudioLimits accepts 0 for every limit, because the API fields are optional.
func (profile ClientProfile) validateAudioLimits() error {
	for _, limit := range []struct {
		name               string
		value, least, most int
	}{
		{name: "max audio channels", value: profile.MaxAudioChannels, least: minProfileChannels, most: maxProfileChannels},
		{
			name: "max audio sample rate", value: profile.MaxAudioSampleRate,
			least: minProfileSampleRate, most: maxProfileSampleRate,
		},
		{
			name: "max audio bit depth", value: profile.MaxAudioBitDepth,
			least: minProfileAudioDepth, most: maxProfileAudioDepth,
		},
	} {
		if limit.value != audioLimitOmitted && (limit.value < limit.least || limit.value > limit.most) {
			return fmt.Errorf("%s %d outside %d..%d: %w", limit.name, limit.value, limit.least, limit.most, domain.ErrInvalid)
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
