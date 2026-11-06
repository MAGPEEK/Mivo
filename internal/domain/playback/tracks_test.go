package playback_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// Stream indexes of the three-audio corpus clip (M4-03): eng stereo default, deu 5.1 and an
// English commentary.
const (
	corpusEnglishIndex    = 1
	corpusGermanIndex     = 2
	corpusCommentaryIndex = 3
	noAudioIndex          = 0
)

func languageAudio(index int, codec, language string, channels int) media.Stream {
	stream := surround(index, codec, channels)
	stream.Language = language

	return stream
}

func titled(stream media.Stream, title string) media.Stream {
	stream.Title = title

	return stream
}

// threeAudioClip is the corpus layout of M4-03.
func threeAudioClip() media.Description {
	return clip(media.ContainerMKV, h264High(0),
		asDefault(languageAudio(corpusEnglishIndex, "aac", "eng", stereoChannels)),
		languageAudio(corpusGermanIndex, "ac3", "deu", surround51),
		titled(languageAudio(corpusCommentaryIndex, "aac", "eng", stereoChannels), "Commentary"))
}

// germanDualLanguageClip is a typical "German.DL" release: German DTS first and default,
// English AC-3 second.
func germanDualLanguageClip() media.Description {
	return clip(media.ContainerMKV, h264High(0),
		asDefault(languageAudio(1, "dts", "ger", surround51)),
		languageAudio(2, "ac3", "eng", surround51))
}

func TestSelectAudio(t *testing.T) {
	t.Parallel()

	commentaryFlag := languageAudio(2, "aac", "deu", surround51)
	commentaryFlag.Commentary = true

	descriptionFlag := languageAudio(2, "aac", "deu", surround51)
	descriptionFlag.VisualImpaired = true

	tests := []struct {
		name        string
		description media.Description
		languages   []string
		wantIndex   int
		wantFound   bool
	}{
		{name: "no audio", description: clip(media.ContainerMKV, h264High(0)), wantIndex: noAudioIndex},
		{name: "corpus without preference takes the default", description: threeAudioClip(), wantIndex: corpusEnglishIndex, wantFound: true},
		{name: "corpus prefers German", description: threeAudioClip(), languages: []string{"de"}, wantIndex: corpusGermanIndex, wantFound: true},
		{name: "corpus prefers English", description: threeAudioClip(), languages: []string{"en"}, wantIndex: corpusEnglishIndex, wantFound: true},
		{name: "corpus unknown language falls back to the default", description: threeAudioClip(), languages: []string{"fr"}, wantIndex: corpusEnglishIndex, wantFound: true},
		{name: "German.DL without preference plays German", description: germanDualLanguageClip(), wantIndex: 1, wantFound: true},
		{name: "German.DL ISO 639-2/T German", description: germanDualLanguageClip(), languages: []string{"deu"}, wantIndex: 1, wantFound: true},
		{name: "German.DL English", description: germanDualLanguageClip(), languages: []string{"eng"}, wantIndex: 2, wantFound: true},
		{name: "list order counts", description: germanDualLanguageClip(), languages: []string{"fr", "EN", "de"}, wantIndex: 2, wantFound: true},
		{name: "region tag matches", description: germanDualLanguageClip(), languages: []string{"en-US"}, wantIndex: 2, wantFound: true},
		{name: "undetermined preference is ignored", description: germanDualLanguageClip(), languages: []string{"und", "en"}, wantIndex: 2, wantFound: true},
		{
			name:        "commentary loses against the second language",
			description: clip(media.ContainerMKV, languageAudio(1, "aac", "deu", stereoChannels), titled(languageAudio(2, "aac", "eng", stereoChannels), "Director's Commentary")),
			languages:   []string{"en", "de"}, wantIndex: 1, wantFound: true,
		},
		{
			name:        "default commentary loses",
			description: clip(media.ContainerMKV, asDefault(titled(languageAudio(1, "aac", "deu", stereoChannels), "Kommentar")), languageAudio(2, "aac", "deu", stereoChannels)),
			wantIndex:   2, wantFound: true,
		},
		{
			name:        "commentary disposition loses",
			description: clip(media.ContainerMKV, asDefault(commentaryFlag), languageAudio(3, "aac", "deu", stereoChannels)),
			wantIndex:   3, wantFound: true,
		},
		{
			name:        "audio description disposition loses",
			description: clip(media.ContainerMKV, asDefault(descriptionFlag), languageAudio(3, "aac", "deu", stereoChannels)),
			wantIndex:   3, wantFound: true,
		},
		{
			name:        "only commentary is still played",
			description: clip(media.ContainerMKV, titled(languageAudio(4, "aac", "eng", stereoChannels), "Commentary")),
			wantIndex:   4, wantFound: true,
		},
		{
			name:        "first wins over more channels without default",
			description: clip(media.ContainerMKV, languageAudio(1, "aac", "eng", stereoChannels), languageAudio(2, "ac3", "eng", surround51)),
			wantIndex:   1, wantFound: true,
		},
		{
			name:        "Norwegian Bokmål matches Norwegian",
			description: clip(media.ContainerMKV, languageAudio(1, "aac", "eng", stereoChannels), languageAudio(2, "aac", "nor", stereoChannels)),
			languages:   []string{"nb"}, wantIndex: 2, wantFound: true,
		},
		{
			name:        "legacy code iw matches Hebrew",
			description: clip(media.ContainerMKV, languageAudio(1, "aac", "eng", stereoChannels), languageAudio(2, "aac", "heb", stereoChannels)),
			languages:   []string{"iw"}, wantIndex: 2, wantFound: true,
		},
		{
			name:        "code outside the table matches itself",
			description: clip(media.ContainerMKV, languageAudio(1, "aac", "eng", stereoChannels), languageAudio(2, "aac", "HAW", stereoChannels)),
			languages:   []string{"haw"}, wantIndex: 2, wantFound: true,
		},
		{
			name:        "first on a tie",
			description: clip(media.ContainerMKV, languageAudio(1, "aac", "eng", stereoChannels), languageAudio(2, "aac", "eng", stereoChannels)),
			wantIndex:   1, wantFound: true,
		},
		{
			name:        "untagged audio never matches",
			description: clip(media.ContainerMKV, asDefault(languageAudio(1, "aac", "", stereoChannels)), languageAudio(2, "aac", "eng", stereoChannels)),
			languages:   []string{"en"}, wantIndex: 2, wantFound: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			index, found := playback.SelectAudio(test.description, playback.TrackPreference{AudioLanguages: test.languages})
			if index != test.wantIndex || found != test.wantFound {
				t.Errorf("SelectAudio = (%d, %v), want (%d, %v)", index, found, test.wantIndex, test.wantFound)
			}
		})
	}
}

func languageSubtitle(index int, codec, language string) media.Stream {
	stream := subtitle(index, codec)
	stream.Language = language

	return stream
}

func forcedSubtitle(index int, language string) media.Stream {
	stream := languageSubtitle(index, "subrip", language)
	stream.Forced = true

	return stream
}

func sdhSubtitle(index int, language string) media.Stream {
	stream := languageSubtitle(index, "subrip", language)
	stream.HearingImpaired = true

	return stream
}

// subtitleClip has German and English full, forced and SDH subtitles.
func subtitleClip() media.Description {
	return clip(media.ContainerMKV, h264High(0), languageAudio(1, "aac", "eng", stereoChannels),
		languageSubtitle(2, "subrip", "ger"),
		forcedSubtitle(3, "ger"),
		languageSubtitle(4, "subrip", "eng"),
		sdhSubtitle(5, "eng"),
		titled(languageSubtitle(6, "subrip", "eng"), "English (Forced)"))
}

func TestSelectSubtitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		description   media.Description
		audioLanguage string
		preference    playback.TrackPreference
		wantID        string
	}{
		{name: "off", description: subtitleClip(), audioLanguage: "ger", preference: playback.TrackPreference{SubtitleMode: playback.SubtitleModeOff}},
		{name: "empty mode shows the forced track of the audio language", description: subtitleClip(), audioLanguage: "de", wantID: "s:3"},
		{name: "forced by title", description: subtitleClip(), audioLanguage: "en", preference: forcedMode(), wantID: "s:6"},
		{name: "no forced track in the audio language", description: subtitleClip(), audioLanguage: "fr", preference: forcedMode()},
		{name: "undetermined audio shows nothing", description: subtitleClip(), audioLanguage: "und", preference: forcedMode()},
		{name: "no subtitles", description: germanDualLanguageClip(), audioLanguage: "de", preference: alwaysMode(false, "de")},
		{name: "always in the preferred language", description: subtitleClip(), audioLanguage: "en", preference: alwaysMode(false, "de"), wantID: "s:2"},
		{name: "always prefers the plain track", description: subtitleClip(), audioLanguage: "de", preference: alwaysMode(false, "en"), wantID: "s:4"},
		{name: "always prefers SDH when asked", description: subtitleClip(), audioLanguage: "de", preference: alwaysMode(true, "en"), wantID: "s:5"},
		{name: "always follows list order", description: subtitleClip(), audioLanguage: "de", preference: alwaysMode(false, "fr", "eng", "de"), wantID: "s:4"},
		{name: "always without a matching language", description: subtitleClip(), audioLanguage: "de", preference: alwaysMode(false, "fr")},
		{
			name: "always falls back to the audio languages", description: subtitleClip(), audioLanguage: "en",
			preference: playback.TrackPreference{AudioLanguages: []string{"de"}, SubtitleMode: playback.SubtitleModeAlways}, wantID: "s:2",
		},
		{name: "always without any language takes the first", description: subtitleClip(), audioLanguage: "en", preference: alwaysMode(false), wantID: "s:2"},
		{
			name: "always prefers the default track", audioLanguage: "en", preference: alwaysMode(false, "en"), wantID: "s:3",
			description: clip(media.ContainerMKV, languageSubtitle(2, "subrip", "eng"), asDefault(languageSubtitle(3, "ass", "eng"))),
		},
		{
			name: "always takes a forced track when nothing else exists", audioLanguage: "en", preference: alwaysMode(false, "en"), wantID: "s:2",
			description: clip(media.ContainerMKV, forcedSubtitle(2, "eng")),
		},
		{
			name: "always prefers a full track of a later language over a forced one", audioLanguage: "en",
			preference: alwaysMode(false, "de", "en"), wantID: "s:3",
			description: clip(media.ContainerMKV, forcedSubtitle(2, "deu"), languageSubtitle(3, "subrip", "eng")),
		},
		{name: "unknown mode shows nothing", description: subtitleClip(), audioLanguage: "de", preference: playback.TrackPreference{SubtitleMode: "sometimes"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			id, found := playback.SelectSubtitle(test.description, test.audioLanguage, test.preference)
			if id != test.wantID || found != (test.wantID != "") {
				t.Errorf("SelectSubtitle = (%q, %v), want %q", id, found, test.wantID)
			}
		})
	}
}

func forcedMode() playback.TrackPreference {
	return playback.TrackPreference{SubtitleMode: playback.SubtitleModeForced}
}

func alwaysMode(preferSDH bool, languages ...string) playback.TrackPreference {
	return playback.TrackPreference{SubtitleLanguages: languages, SubtitleMode: playback.SubtitleModeAlways, PreferSDH: preferSDH}
}

func TestSubtitleTracks(t *testing.T) {
	t.Parallel()

	description := clip(media.ContainerMKV, h264High(0), audio(1, "aac"),
		asDefault(titled(languageSubtitle(2, "subrip", "eng"), "English SDH")),
		forcedSubtitle(4, "deu"),
		languageSubtitle(7, "hdmv_pgs_subtitle", "fra"))

	want := []playback.SubtitleTrack{
		{ID: "s:2", Language: "eng", Title: "English SDH", Format: "subrip", SDH: true, Default: true, TextBased: true},
		{ID: "s:4", Language: "deu", Format: "subrip", Forced: true, TextBased: true},
		{ID: "s:7", Language: "fra", Format: "hdmv_pgs_subtitle"},
	}

	if got := playback.SubtitleTracks(description); !slices.Equal(got, want) {
		t.Errorf("SubtitleTracks = %+v, want %+v", got, want)
	}
}

func TestSelectSubtitleTrackIncludesExternalFiles(t *testing.T) {
	t.Parallel()

	tracks := []playback.SubtitleTrack{
		{ID: "s:2", Language: "eng", TextBased: true},
		{ID: "x:0a1b2c", Language: "deu", External: true, TextBased: true},
	}

	id, found := playback.SelectSubtitleTrack(tracks, "en", alwaysMode(false, "de"))
	if id != "x:0a1b2c" || !found {
		t.Errorf("SelectSubtitleTrack = (%q, %v), want the external German file", id, found)
	}
}

func TestTrackPreferenceValidate(t *testing.T) {
	t.Parallel()

	tooMany := []string{"de", "en", "fr", "es", "it", "nl", "sv", "da", "fi"}

	tests := []struct {
		name       string
		preference playback.TrackPreference
		wantErr    bool
	}{
		{name: "zero value"},
		{name: "valid tags", preference: playback.TrackPreference{
			AudioLanguages: []string{"de", "eng", "pt-BR", "zh_Hant", "es-419"}, SubtitleLanguages: []string{"DE"},
			SubtitleMode: playback.SubtitleModeAlways,
		}},
		{name: "every mode", preference: playback.TrackPreference{SubtitleMode: playback.SubtitleModeOff}},
		{name: "forced mode", preference: forcedMode()},
		{name: "unknown mode", preference: playback.TrackPreference{SubtitleMode: "sometimes"}, wantErr: true},
		{name: "too many audio languages", preference: playback.TrackPreference{AudioLanguages: tooMany}, wantErr: true},
		{name: "too many subtitle languages", preference: playback.TrackPreference{SubtitleLanguages: tooMany}, wantErr: true},
		{name: "empty tag", preference: playback.TrackPreference{AudioLanguages: []string{""}}, wantErr: true},
		{name: "one letter", preference: playback.TrackPreference{AudioLanguages: []string{"d"}}, wantErr: true},
		{name: "four letters", preference: playback.TrackPreference{AudioLanguages: []string{"deut"}}, wantErr: true},
		{name: "digit in primary", preference: playback.TrackPreference{AudioLanguages: []string{"d1"}}, wantErr: true},
		{name: "empty subtag", preference: playback.TrackPreference{AudioLanguages: []string{"pt--br"}}, wantErr: true},
		{name: "trailing separator", preference: playback.TrackPreference{AudioLanguages: []string{"pt-"}}, wantErr: true},
		{name: "punctuation in subtag", preference: playback.TrackPreference{SubtitleLanguages: []string{"pt-b!"}}, wantErr: true},
		{name: "too long", preference: playback.TrackPreference{SubtitleLanguages: []string{"de-aaaaaaaa-bbbbbbbb"}}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := test.preference.Validate()
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate() = %v, want error %v", err, test.wantErr)
			}

			if err != nil && !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("error %v does not wrap ErrInvalid", err)
			}

			profile := playback.ClientProfile{HLS: playback.HLSProfile{Delivery: playback.HLSDeliveryMSE}, Tracks: test.preference}
			if profileErr := profile.Validate(); (profileErr != nil) != test.wantErr {
				t.Errorf("ClientProfile.Validate() = %v, want error %v", profileErr, test.wantErr)
			}
		})
	}
}

func TestEmbeddedSubtitleTrackID(t *testing.T) {
	t.Parallel()

	if got := playback.EmbeddedSubtitleTrackID(12); got != "s:12" {
		t.Errorf("EmbeddedSubtitleTrackID(12) = %q, want s:12", got)
	}
}
