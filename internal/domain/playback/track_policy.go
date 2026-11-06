package playback

import "github.com/magpeek/mivo/internal/domain/media"

// audioRank orders audio streams for the default choice (ADR 0030 §4). Commentary and audio
// description come before the language on purpose: a commentary in the first preferred
// language must not win over the film in the second one. Channel counts do not rank: on a tie
// the first track wins, because browsers play only the first track of a file, so "stereo
// first, 5.1 second" keeps direct play (ADR 0023, Notes M4-04a).
type audioRank struct {
	commentary bool
	language   int
	notDefault bool
}

func (rank audioRank) betterThan(other audioRank) bool {
	switch {
	case rank.commentary != other.commentary:
		return !rank.commentary
	case rank.language != other.language:
		return rank.language < other.language
	default:
		return !rank.notDefault && other.notDefault
	}
}

// SelectAudio returns the ffprobe index of the audio stream to play without a user choice:
// no commentary or audio description, then the earliest preferred language, then the
// default disposition, then file order. The second result is false
// when the file has no audio.
func SelectAudio(description media.Description, preference TrackPreference) (int, bool) {
	stream, found := selectAudioStream(description, preference)

	return stream.Index, found
}

func selectAudioStream(description media.Description, preference TrackPreference) (media.Stream, bool) {
	var (
		best     media.Stream
		bestRank audioRank
		found    bool
	)

	for _, stream := range description.Streams {
		if stream.Kind != media.StreamKindAudio {
			continue
		}

		rank := audioRank{
			commentary: isCommentary(stream),
			language:   languageRank(stream.Language, preference.AudioLanguages),
			notDefault: !stream.Default,
		}

		if !found || rank.betterThan(bestRank) {
			best, bestRank, found = stream, rank, true
		}
	}

	return best, found
}

// subtitleRank orders the candidate subtitles of one mode: full tracks before forced-only
// ones (so "always" never shows a forced track while a full one exists), then the earlier
// language, the wanted SDH kind, the default disposition and file order.
type subtitleRank struct {
	language   int
	forced     bool
	sdhMissed  bool
	notDefault bool
}

func (rank subtitleRank) betterThan(other subtitleRank) bool {
	switch {
	case rank.forced != other.forced:
		return !rank.forced
	case rank.language != other.language:
		return rank.language < other.language
	case rank.sdhMissed != other.sdhMissed:
		return !rank.sdhMissed
	default:
		return !rank.notDefault && other.notDefault
	}
}

// SelectSubtitle returns the id of the embedded subtitle to show without a user choice, for
// audio in audioLanguage; see SelectSubtitleTrack. The second result is false for "off".
func SelectSubtitle(description media.Description, audioLanguage string, preference TrackPreference) (string, bool) {
	return SelectSubtitleTrack(SubtitleTracks(description), audioLanguage, preference)
}

// SelectSubtitleTrack picks from embedded and external tracks (ADR 0030 §3):
//   - SubtitleModeOff shows nothing.
//   - SubtitleModeForced (and an empty mode) shows a forced track in the audio language, the
//     one that translates foreign-language parts; an undetermined audio language shows nothing.
//   - SubtitleModeAlways shows a full track in the earliest preferred subtitle language
//     (empty list: the audio languages), the SDH kind the preference asks for when there
//     is a choice; without any language preference the first track qualifies. A forced
//     track is only taken when no full track of a preferred language exists.
//
// The second result is false for "off".
func SelectSubtitleTrack(tracks []SubtitleTrack, audioLanguage string, preference TrackPreference) (string, bool) {
	switch preference.SubtitleMode {
	case SubtitleModeOff:
		return "", false
	case SubtitleModeAlways:
		return bestSubtitle(tracks, preference.alwaysLanguages(), preference.PreferSDH, false)
	case "", SubtitleModeForced:
		return bestSubtitle(tracks, []string{audioLanguage}, preference.PreferSDH, true)
	default:
		return "", false
	}
}

// alwaysLanguages are the languages of full subtitles: the subtitle list, else the audio list.
func (preference TrackPreference) alwaysLanguages() []string {
	if len(preference.SubtitleLanguages) > 0 {
		return preference.SubtitleLanguages
	}

	return preference.AudioLanguages
}

// bestSubtitle returns the best ranked track in one of the languages; an empty language
// list accepts every language. forcedOnly keeps forced tracks only.
func bestSubtitle(tracks []SubtitleTrack, languages []string, preferSDH, forcedOnly bool) (string, bool) {
	var (
		bestID   string
		bestRank subtitleRank
		found    bool
	)

	for _, track := range tracks {
		rank := subtitleRank{
			language:   languageRank(track.Language, languages),
			forced:     track.Forced,
			sdhMissed:  track.SDH != preferSDH,
			notDefault: !track.Default,
		}

		languageMissed := len(languages) > 0 && rank.language == len(languages)
		if languageMissed || (forcedOnly && !track.Forced) {
			continue
		}

		if !found || rank.betterThan(bestRank) {
			bestID, bestRank, found = track.ID, rank, true
		}
	}

	return bestID, found
}
