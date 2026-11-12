package library_test

import (
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

func textSubtitle(path string, format media.SubtitleFormat, language string) media.ExternalSubtitle {
	return media.ExternalSubtitle{RelativePath: path, Format: format, Language: language, TextBased: true}
}

func imageSubtitle(path string, format media.SubtitleFormat, language string) media.ExternalSubtitle {
	return media.ExternalSubtitle{RelativePath: path, Format: format, Language: language}
}

func withFlags(subtitle media.ExternalSubtitle, forced, sdh, isDefault bool) media.ExternalSubtitle {
	subtitle.Forced, subtitle.SDH, subtitle.Default = forced, sdh, isDefault

	return subtitle
}

func withTitle(subtitle media.ExternalSubtitle, title string) media.ExternalSubtitle {
	subtitle.Title = title

	return subtitle
}

func TestParseSubtitleNameLanguageAndFlags(t *testing.T) {
	t.Parallel()

	const srt = media.SubtitleFormatSubRip

	tests := []struct {
		name     string
		fileName string
		want     media.ExternalSubtitle
	}{
		{name: "two-letter language", fileName: "Movie.en.srt", want: textSubtitle("Movie.en.srt", srt, "en")},
		{
			name: "three-letter language and forced", fileName: "Movie.eng.forced.srt",
			want: withFlags(textSubtitle("Movie.eng.forced.srt", srt, "eng"), true, false, false),
		},
		{
			name: "region tag and SDH", fileName: "Movie.pt-BR.sdh.ass",
			want: withFlags(textSubtitle("Movie.pt-BR.sdh.ass", media.SubtitleFormatASS, "pt-BR"), false, true, false),
		},
		{name: "no language", fileName: "Movie.srt", want: textSubtitle("Movie.srt", srt, "und")},
		{name: "explicit undetermined", fileName: "Movie.und.srt", want: textSubtitle("Movie.und.srt", srt, "und")},
		{name: "bibliographic code", fileName: "Movie.ger.srt", want: textSubtitle("Movie.ger.srt", srt, "ger")},
		{name: "upper-case language", fileName: "Movie.DE.srt", want: textSubtitle("Movie.DE.srt", srt, "de")},
		{name: "underscore region", fileName: "Movie.pt_br.srt", want: textSubtitle("Movie.pt_br.srt", srt, "pt-BR")},
		{name: "script subtag", fileName: "Movie.zh-hans.srt", want: textSubtitle("Movie.zh-hans.srt", srt, "zh-Hans")},
		{name: "numeric region", fileName: "Movie.es-419.srt", want: textSubtitle("Movie.es-419.srt", srt, "es-419")},
		{
			name: "script and region", fileName: "Movie.zh-Hant-TW.srt",
			want: textSubtitle("Movie.zh-Hant-TW.srt", srt, "zh-Hant-TW"),
		},
		{
			name: "flags before the language", fileName: "Movie.forced.de.srt",
			want: withFlags(textSubtitle("Movie.forced.de.srt", srt, "de"), true, false, false),
		},
		{
			name: "all flags, any case", fileName: "Movie.en.Default.FORCED.SDH.srt",
			want: withFlags(textSubtitle("Movie.en.Default.FORCED.SDH.srt", srt, "en"), true, true, true),
		},
		{
			name: "closed captions are SDH", fileName: "Movie.en.cc.srt",
			want: withFlags(textSubtitle("Movie.en.cc.srt", srt, "en"), false, true, false),
		},
		{
			name: "foreign means forced", fileName: "Movie.en.foreign.srt",
			want: withFlags(textSubtitle("Movie.en.foreign.srt", srt, "en"), true, false, false),
		},
		{name: "hi alone is Hindi", fileName: "Movie.hi.srt", want: textSubtitle("Movie.hi.srt", srt, "hi")},
		{
			name: "hi next to a language is hearing impaired", fileName: "Movie.en.hi.srt",
			want: withFlags(textSubtitle("Movie.en.hi.srt", srt, "en"), false, true, false),
		},
		{
			name: "hi before a language is hearing impaired", fileName: "Movie.hi.en.srt",
			want: withFlags(textSubtitle("Movie.hi.en.srt", srt, "en"), false, true, false),
		},
		{
			name: "flag without language", fileName: "Movie.forced.srt",
			want: withFlags(textSubtitle("Movie.forced.srt", srt, "und"), true, false, false),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, ok := library.ParseSubtitleName("Movie", test.fileName)
			if !ok {
				t.Fatalf("ParseSubtitleName(%q) was not recognised", test.fileName)
			}

			if got != test.want {
				t.Errorf("ParseSubtitleName(%q) = %+v, want %+v", test.fileName, got, test.want)
			}
		})
	}
}

func TestParseSubtitleNameTitle(t *testing.T) {
	t.Parallel()

	const srt = media.SubtitleFormatSubRip

	tests := []struct {
		name     string
		fileName string
		want     media.ExternalSubtitle
	}{
		{
			name: "unknown token after the language", fileName: "Movie.en.commentary.srt",
			want: withTitle(textSubtitle("Movie.en.commentary.srt", srt, "en"), "commentary"),
		},
		{
			name: "unknown tokens before the language", fileName: "Movie.Director Notes.v2.de.srt",
			want: withTitle(textSubtitle("Movie.Director Notes.v2.de.srt", srt, "de"), "Director Notes v2"),
		},
		{
			name: "only the last language counts", fileName: "Movie.it.follows.en.srt",
			want: withTitle(textSubtitle("Movie.it.follows.en.srt", srt, "en"), "it follows"),
		},
		{
			name: "unknown three-letter token", fileName: "Movie.xyz.srt",
			want: withTitle(textSubtitle("Movie.xyz.srt", srt, "und"), "xyz"),
		},
		{
			name: "unknown subtag keeps the token in the title", fileName: "Movie.en-commentary.srt",
			want: withTitle(textSubtitle("Movie.en-commentary.srt", srt, "und"), "en-commentary"),
		},
		{
			name: "too many subtags", fileName: "Movie.zh-Hant-TW-x.srt",
			want: withTitle(textSubtitle("Movie.zh-Hant-TW-x.srt", srt, "und"), "zh-Hant-TW-x"),
		},
		{
			name: "non-ASCII token", fileName: "Movie.dé.srt",
			want: withTitle(textSubtitle("Movie.dé.srt", srt, "und"), "dé"),
		},
		{
			name: "mixed letters and digits subtag", fileName: "Movie.en-4a9.srt",
			want: withTitle(textSubtitle("Movie.en-4a9.srt", srt, "und"), "en-4a9"),
		},
		{name: "empty tokens are ignored", fileName: "Movie..en..srt", want: textSubtitle("Movie..en..srt", srt, "en")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, ok := library.ParseSubtitleName("Movie", test.fileName)
			if !ok {
				t.Fatalf("ParseSubtitleName(%q) was not recognised", test.fileName)
			}

			if got != test.want {
				t.Errorf("ParseSubtitleName(%q) = %+v, want %+v", test.fileName, got, test.want)
			}
		})
	}
}

func TestParseSubtitleNameFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fileName string
		want     media.ExternalSubtitle
	}{
		{fileName: "Movie.en.srt", want: textSubtitle("Movie.en.srt", media.SubtitleFormatSubRip, "en")},
		{fileName: "Movie.en.ass", want: textSubtitle("Movie.en.ass", media.SubtitleFormatASS, "en")},
		{fileName: "Movie.en.ssa", want: textSubtitle("Movie.en.ssa", media.SubtitleFormatSSA, "en")},
		{fileName: "Movie.en.vtt", want: textSubtitle("Movie.en.vtt", media.SubtitleFormatWebVTT, "en")},
		{fileName: "Movie.en.idx", want: imageSubtitle("Movie.en.idx", media.SubtitleFormatVobSub, "en")},
		{fileName: "Movie.en.sub", want: imageSubtitle("Movie.en.sub", media.SubtitleFormatVobSub, "en")},
		{fileName: "Movie.en.sup", want: imageSubtitle("Movie.en.sup", media.SubtitleFormatPGS, "en")},
		{fileName: "Movie.en.SRT", want: textSubtitle("Movie.en.SRT", media.SubtitleFormatSubRip, "en")},
		{fileName: "Movie.en.Sup", want: imageSubtitle("Movie.en.Sup", media.SubtitleFormatPGS, "en")},
	}

	for _, test := range tests {
		t.Run(test.fileName, func(t *testing.T) {
			t.Parallel()

			got, ok := library.ParseSubtitleName("Movie", test.fileName)
			if !ok {
				t.Fatalf("ParseSubtitleName(%q) was not recognised", test.fileName)
			}

			if got != test.want {
				t.Errorf("ParseSubtitleName(%q) = %+v, want %+v", test.fileName, got, test.want)
			}
		})
	}
}

func TestParseSubtitleNameBaseAndFolder(t *testing.T) {
	t.Parallel()

	const srt = media.SubtitleFormatSubRip

	tests := []struct {
		name      string
		videoBase string
		fileName  string
		want      media.ExternalSubtitle
	}{
		{
			name: "Subs folder", videoBase: "Movie", fileName: "Subs/Movie.es.srt",
			want: textSubtitle("Subs/Movie.es.srt", srt, "es"),
		},
		{
			name: "folder name in any case", videoBase: "Movie", fileName: "SUBS/Movie.es.srt",
			want: textSubtitle("SUBS/Movie.es.srt", srt, "es"),
		},
		{
			name: "Subtitles folder", videoBase: "Movie", fileName: "Subtitles/Movie.srt",
			want: textSubtitle("Subtitles/Movie.srt", srt, "und"),
		},
		{
			name: "base in another case", videoBase: "Movie (2020)", fileName: "movie (2020).EN.srt",
			want: textSubtitle("movie (2020).EN.srt", srt, "en"),
		},
		{
			name: "NFC base with non-ASCII letters", videoBase: "Am\u00e9lie \u00dcber", fileName: "AM\u00c9LIE \u00fcber.fr.srt",
			want: textSubtitle("AM\u00c9LIE \u00fcber.fr.srt", srt, "fr"),
		},
		{
			name: "base with dots", videoBase: "Show.S01E02.1080p", fileName: "Show.S01E02.1080p.de.forced.srt",
			want: withFlags(textSubtitle("Show.S01E02.1080p.de.forced.srt", srt, "de"), true, false, false),
		},
		{
			name: "case folding that changes the byte length", videoBase: "\u212aelvin", fileName: "kelvin.en.srt",
			want: textSubtitle("kelvin.en.srt", srt, "en"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, ok := library.ParseSubtitleName(test.videoBase, test.fileName)
			if !ok {
				t.Fatalf("ParseSubtitleName(%q, %q) was not recognised", test.videoBase, test.fileName)
			}

			if got != test.want {
				t.Errorf("ParseSubtitleName(%q, %q) = %+v, want %+v", test.videoBase, test.fileName, got, test.want)
			}
		})
	}
}

func TestParseSubtitleNameRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		videoBase string
		fileName  string
	}{
		{name: "other base name", videoBase: "Movie", fileName: "Other.en.srt"},
		{name: "base is only a prefix", videoBase: "Movie", fileName: "Movie 2.en.srt"},
		{name: "base is longer than the name", videoBase: "Movie Extended", fileName: "Movie.srt"},
		{name: "decomposed twin of an NFC base", videoBase: "Am\u00e9lie", fileName: "Ame\u0301lie.fr.srt"},
		{name: "not a subtitle", videoBase: "Movie", fileName: "Movie.en.txt"},
		{name: "artwork", videoBase: "Movie", fileName: "Movie.jpg"},
		{name: "the video itself", videoBase: "Movie", fileName: "Movie.mkv"},
		{name: "no extension", videoBase: "Movie", fileName: "Movie"},
		{name: "extension only", videoBase: "Movie", fileName: ".srt"},
		{name: "empty base", videoBase: "", fileName: ".en.srt"},
		{name: "empty file name", videoBase: "Movie", fileName: ""},
		{name: "other folder", videoBase: "Movie", fileName: "Extras/Movie.en.srt"},
		{name: "nested folder", videoBase: "Movie", fileName: "Subs/Movie/Movie.en.srt"},
		{name: "folder above a subtitle folder", videoBase: "Movie", fileName: "Other/Subs/Movie.en.srt"},
		{name: "parent folder", videoBase: "Movie", fileName: "../Movie.en.srt"},
		{name: "parent of the subtitle folder", videoBase: "Movie", fileName: "Subs/../Movie.en.srt"},
		{name: "absolute path", videoBase: "Movie", fileName: "/Subs/Movie.en.srt"},
		{name: "drive path", videoBase: "Movie", fileName: "C:/Movie.en.srt"},
		{name: "backslash", videoBase: "Movie", fileName: "Subs\\Movie.en.srt"},
		{name: "NUL byte", videoBase: "Movie", fileName: "Movie.en\x00.srt"},
		{name: "base that leaves the folder", videoBase: "../Movie", fileName: "../Movie.en.srt"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got, ok := library.ParseSubtitleName(test.videoBase, test.fileName); ok {
				t.Errorf("ParseSubtitleName(%q, %q) = %+v, want it rejected", test.videoBase, test.fileName, got)
			}
		})
	}
}

func TestExternalSubtitles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fileNames []string
		want      []string
	}{
		{name: "nothing", fileNames: nil, want: nil},
		{
			name:      "sorted by path, other files skipped",
			fileNames: []string{"Movie.mkv", "Subs/Movie.es.srt", "Movie.en.srt", "Other.en.srt", "Movie.de.ass"},
			want:      []string{"Movie.de.ass", "Movie.en.srt", "Subs/Movie.es.srt"},
		},
		{
			name:      "sub and idx are one track, named by the index",
			fileNames: []string{"Movie.en.sub", "Movie.en.idx"},
			want:      []string{"Movie.en.idx"},
		},
		{
			name:      "pair with extensions in another case",
			fileNames: []string{"Movie.en.IDX", "Movie.en.Sub"},
			want:      []string{"Movie.en.IDX"},
		},
		{name: "index without pictures", fileNames: []string{"Movie.en.idx"}, want: nil},
		{name: "pictures without index", fileNames: []string{"Movie.en.sub"}, want: nil},
		{
			name:      "halves in different folders are no pair",
			fileNames: []string{"Movie.en.idx", "Subs/Movie.en.sub"},
			want:      nil,
		},
		{
			name:      "halves of different languages are no pair",
			fileNames: []string{"Movie.en.idx", "Movie.de.sub", "Movie.de.sup"},
			want:      []string{"Movie.de.sup"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var got []string
			for _, subtitle := range library.ExternalSubtitles("Movie", test.fileNames) {
				got = append(got, subtitle.RelativePath)
			}

			if !slices.Equal(got, test.want) {
				t.Errorf("ExternalSubtitles(%q) = %q, want %q", test.fileNames, got, test.want)
			}
		})
	}
}

func TestExternalSubtitlesKeepsTheParsedTrack(t *testing.T) {
	t.Parallel()

	got := library.ExternalSubtitles("Movie", []string{"Movie.en.forced.sub", "Movie.en.forced.idx"})
	want := withFlags(imageSubtitle("Movie.en.forced.idx", media.SubtitleFormatVobSub, "en"), true, false, false)

	if len(got) != 1 || got[0] != want {
		t.Errorf("ExternalSubtitles = %+v, want [%+v]", got, want)
	}
}

func TestExternalSubtitlesDoesNotChangeTheInput(t *testing.T) {
	t.Parallel()

	fileNames := []string{"Movie.en.srt", "Movie.de.srt"}
	library.ExternalSubtitles("Movie", fileNames)

	if want := []string{"Movie.en.srt", "Movie.de.srt"}; !slices.Equal(fileNames, want) {
		t.Errorf("input = %q, want it unchanged %q", fileNames, want)
	}
}
