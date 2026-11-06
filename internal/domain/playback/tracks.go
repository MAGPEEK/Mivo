package playback

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

// maxPreferredLanguages bounds each language list of a track preference.
const maxPreferredLanguages = 8

// embeddedTrackPrefix starts the track id of a subtitle stream inside the media file
// (api/openapi.yaml, M4-02a): "s:<ffprobe index>". External files use "x:<hash>".
const embeddedTrackPrefix = "s:"

// externalTrackPrefix starts the track id of a subtitle file next to the media file (M4-05b).
const externalTrackPrefix = "x:"

// maxExternalSubtitles bounds the subtitle files of one media file a decision considers.
const maxExternalSubtitles = 64

// Title words that mark tracks whose disposition flag is missing; muxers often only name them.
// Matching is on the lower-case title, so "Director's Commentary" and "Kommentar" both count.
func commentaryTitleWords() []string {
	return []string{"comment", "kommentar", "description", "descriptive", "audiodeskription", "hörfilm"}
}

func forcedTitleWords() []string { return []string{"forced", "erzwungen"} }

func hearingImpairedTitleWords() []string {
	return []string{"sdh", "hearing impaired", "hörgeschädigt"}
}

// SubtitleMode says when a subtitle is switched on without a choice of the user.
type SubtitleMode string

// The subtitle modes of ADR 0030 §3.
const (
	// SubtitleModeOff never switches a subtitle on.
	SubtitleModeOff SubtitleMode = "off"
	// SubtitleModeForced shows a forced track (translations of foreign-language parts) in
	// the language of the audio track; it is the mode of an empty preference.
	SubtitleModeForced SubtitleMode = "forced"
	// SubtitleModeAlways shows a full track in a preferred subtitle language.
	SubtitleModeAlways SubtitleMode = "always"
)

func (mode SubtitleMode) isKnown() bool {
	switch mode {
	case "", SubtitleModeOff, SubtitleModeForced, SubtitleModeAlways:
		return true
	default:
		return false
	}
}

// TrackPreference is how a profile wants tracks picked when the request names none
// (ADR 0030 §3, §4). The zero value prefers no language and shows forced tracks only.
type TrackPreference struct {
	// AudioLanguages are ISO 639-1/-2 tags, most wanted first.
	AudioLanguages []string
	// SubtitleLanguages are the languages of full subtitles in SubtitleModeAlways, most
	// wanted first; empty falls back to AudioLanguages.
	SubtitleLanguages []string
	// SubtitleMode is empty or one of the SubtitleMode constants; empty means forced.
	SubtitleMode SubtitleMode
	// PreferSDH picks subtitles for the deaf and hard of hearing over plain ones.
	PreferSDH bool
}

// Validate bounds the language lists and checks every tag and the mode. Errors wrap
// domain.ErrInvalid.
func (preference TrackPreference) Validate() error {
	for _, list := range []struct {
		name      string
		languages []string
	}{
		{name: "audio languages", languages: preference.AudioLanguages},
		{name: "subtitle languages", languages: preference.SubtitleLanguages},
	} {
		if err := validateCount(list.name, len(list.languages), maxPreferredLanguages); err != nil {
			return err
		}

		for _, language := range list.languages {
			if !isLanguageTag(language) {
				return fmt.Errorf("%s: %q is no language tag: %w", list.name, language, domain.ErrInvalid)
			}
		}
	}

	if !preference.SubtitleMode.isKnown() {
		return fmt.Errorf("unknown subtitle mode %q: %w", preference.SubtitleMode, domain.ErrInvalid)
	}

	return nil
}

// SubtitleTrack is a subtitle the player can show: a stream of the file or an external file.
type SubtitleTrack struct {
	// ID is stable for the file: "s:<index>" for embedded streams, "x:<hash>" for files.
	ID       string
	Language string
	Title    string
	// Format is the codec or file format, for example subrip, ass or hdmv_pgs_subtitle.
	Format    string
	Forced    bool
	SDH       bool
	Default   bool
	External  bool
	TextBased bool
}

// EmbeddedSubtitleTrackID is the track id of the subtitle stream with the ffprobe index.
func EmbeddedSubtitleTrackID(streamIndex int) string {
	return embeddedTrackPrefix + strconv.Itoa(streamIndex)
}

// SubtitleTracks lists the subtitle streams of the file in file order. Forced and SDH come
// from the disposition or, when a muxer only named them, from the title.
func SubtitleTracks(description media.Description) []SubtitleTrack {
	var tracks []SubtitleTrack

	for _, stream := range description.Streams {
		if stream.Kind != media.StreamKindSubtitle {
			continue
		}

		tracks = append(tracks, SubtitleTrack{
			ID:        EmbeddedSubtitleTrackID(stream.Index),
			Language:  stream.Language,
			Title:     stream.Title,
			Format:    stream.Codec,
			Forced:    stream.Forced || titleHasWord(stream.Title, forcedTitleWords()),
			SDH:       stream.HearingImpaired || titleHasWord(stream.Title, hearingImpairedTitleWords()),
			Default:   stream.Default,
			TextBased: !stream.IsBitmapSubtitle(),
		})
	}

	return tracks
}

// isCommentary reports commentary and audio description tracks, which nobody wants by default.
func isCommentary(stream media.Stream) bool {
	return stream.Commentary || stream.VisualImpaired || titleHasWord(stream.Title, commentaryTitleWords())
}

func titleHasWord(title string, words []string) bool {
	lower := strings.ToLower(title)
	for _, word := range words {
		if strings.Contains(lower, word) {
			return true
		}
	}

	return false
}

// findSubtitleTrack returns the track with the id; an unknown id wraps domain.ErrInvalid,
// because the request names it (HTTP 400, not 404).
func findSubtitleTrack(tracks []SubtitleTrack, id string) (SubtitleTrack, error) {
	for _, track := range tracks {
		if track.ID == id {
			return track, nil
		}
	}

	return SubtitleTrack{}, fmt.Errorf("unknown subtitle track %q: %w", id, domain.ErrInvalid)
}

// validateExternalSubtitles bounds the external tracks of one file and checks that every id
// is an external one ("x:...") and unique, so a request id names at most one track.
func validateExternalSubtitles(tracks []SubtitleTrack) error {
	if err := validateCount("external subtitles", len(tracks), maxExternalSubtitles); err != nil {
		return err
	}

	seen := make(map[string]bool, len(tracks))

	for _, track := range tracks {
		if len(track.ID) <= len(externalTrackPrefix) || !strings.HasPrefix(track.ID, externalTrackPrefix) {
			return fmt.Errorf("external subtitle id %q does not start with %q: %w",
				track.ID, externalTrackPrefix, domain.ErrInvalid)
		}

		if seen[track.ID] {
			return fmt.Errorf("duplicate external subtitle id %q: %w", track.ID, domain.ErrInvalid)
		}

		seen[track.ID] = true
	}

	return nil
}
