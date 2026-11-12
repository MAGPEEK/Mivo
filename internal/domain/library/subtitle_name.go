package library

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/magpeek/mivo/internal/domain/media"
)

const (
	subtitleTokenSeparator   = "."
	subtitleTitleSeparator   = " "
	languageSubtagSeparator  = "-"
	alternateSubtagSeparator = "_"
	// hearingImpairedToken is "hi": Hindi on its own, "hearing impaired" next to a language.
	hearingImpairedToken = "hi"
	scriptSubtagLength   = 4
	numericRegionLength  = 3
	vobSubIndexExtension = ".idx"
	vobSubImageExtension = ".sub"
	noLanguageToken      = -1
)

// subtitleLanguageCodes lists the codes a subtitle name may use for its language: every
// ISO 639-1 code with the ISO 639-2 codes (bibliographic and terminologic) of the same
// language, the deprecated "iw", and "fil", "mul" and "und". Languages that only have a
// three-letter code are left out on purpose: many of those codes are everyday words ("cut",
// "new", "war") that belong to a title. It is a constant instead of a set so the package
// keeps no global state; the surrounding spaces make every code findable as " code ".
const subtitleLanguageCodes = " aa aar ab abk ae ave af afr ak aka am amh an arg ar ara as asm av ava ay aym" +
	" az aze ba bak be bel bg bul bh bih bi bis bm bam bn ben bo tib bod br bre bs bos ca cat" +
	" ce che ch cha co cos cr cre cs cze ces cu chu cv chv cy wel cym da dan de ger deu dv div" +
	" dz dzo ee ewe el gre ell en eng eo epo es spa et est eu baq eus fa per fas ff ful fi fin" +
	" fj fij fo fao fr fre fra fy fry ga gle gd gla gl glg gn grn gu guj gv glv ha hau he iw heb" +
	" hi hin ho hmo hr hrv ht hat hu hun hy arm hye hz her ia ina id ind ie ile ig ibo ii iii" +
	" ik ipk io ido is ice isl it ita iu iku ja jpn jv jav ka geo kat kg kon ki kik kj kua" +
	" kk kaz kl kal km khm kn kan ko kor kr kau ks kas ku kur kv kom kw cor ky kir la lat" +
	" lb ltz lg lug li lim ln lin lo lao lt lit lu lub lv lav mg mlg mh mah mi mao mri" +
	" mk mac mkd ml mal mn mon mr mar ms may msa mt mlt my bur mya na nau nb nob nd nde ne nep" +
	" ng ndo nl dut nld nn nno no nor nr nbl nv nav ny nya oc oci oj oji om orm or ori os oss" +
	" pa pan pi pli pl pol ps pus pt por qu que rm roh rn run ro rum ron ru rus rw kin sa san" +
	" sc srd sd snd se sme sg sag si sin sk slo slk sl slv sm smo sn sna so som sq alb sqi" +
	" sr srp ss ssw st sot su sun sv swe sw swa ta tam te tel tg tgk th tha ti tir tk tuk" +
	" tl tgl tn tsn to ton tr tur ts tso tt tat tw twi ty tah ug uig uk ukr ur urd uz uzb" +
	" ve ven vi vie vo vol wa wln wo wol xh xho yi yid yo yor za zha zh chi zho zu zul" +
	" fil mul und "

// subtitleFolders are the folders below a video's folder that hold its subtitle files,
// in lower case.
func subtitleFolders() []string {
	return []string{"subs", "subtitles"}
}

// ParseSubtitleName reads what the name of a file next to a video says about it as a
// subtitle (ADR 0030 §3). videoBase is the video's file name without its extension;
// fileName is the candidate relative to the video's folder, either a plain name or one
// level down in a "Subs" or "Subtitles" folder. Both enter in Unicode NFC (package rule);
// base, folder, extension, language and flags match in any case.
//
// The name must be the video base followed by nothing or by dot-separated tokens and a
// subtitle extension: "Movie.srt", "Movie.eng.forced.srt", "Subs/Movie.pt-BR.sdh.ass". The
// last token that is a language code gives the language ("und" when there is none); forced,
// foreign, sdh, cc, hi and default set the flags; every other token stays in Title. A name
// that belongs to another video, has no subtitle extension, or is not a safe relative path
// reports false.
//
// When one video base is the prefix of another ("Movie" and "Movie.2"), a file can match
// both; the caller gives it to the longest matching base.
func ParseSubtitleName(videoBase, fileName string) (media.ExternalSubtitle, bool) {
	if videoBase == "" || checkRelativePath(fileName) != nil {
		return media.ExternalSubtitle{}, false
	}

	name, inPlace := cutSubtitleFolder(fileName)
	extensionStart := strings.LastIndex(name, subtitleTokenSeparator)

	if !inPlace || extensionStart < 0 {
		return media.ExternalSubtitle{}, false
	}

	format, isSubtitle := media.SubtitleFormatForExtension(name[extensionStart:])
	tokens, matches := cutVideoBase(name[:extensionStart], videoBase)

	if !isSubtitle || !matches {
		return media.ExternalSubtitle{}, false
	}

	subtitle := media.ExternalSubtitle{
		RelativePath: fileName,
		Format:       format,
		Language:     media.UndeterminedLanguage,
		TextBased:    format.TextBased(),
	}

	return withNameTokens(subtitle, tokens), true
}

// ExternalSubtitles returns the subtitle tracks of the video among the files of its folder
// and its subtitle folders, sorted by path so the order never depends on the directory
// listing. A VobSub subtitle is one track for its two files and carries the path of the
// .idx file, the one FFmpeg opens; an .idx or .sub file without its other half is skipped.
func ExternalSubtitles(videoBase string, fileNames []string) []media.ExternalSubtitle {
	var subtitles []media.ExternalSubtitle

	for _, fileName := range fileNames {
		subtitle, ok := ParseSubtitleName(videoBase, fileName)
		if !ok || (subtitle.Format == media.SubtitleFormatVobSub && !isPairedVobSubIndex(fileName, fileNames)) {
			continue
		}

		subtitles = append(subtitles, subtitle)
	}

	slices.SortFunc(subtitles, func(left, right media.ExternalSubtitle) int {
		return strings.Compare(left.RelativePath, right.RelativePath)
	})

	return subtitles
}

// isPairedVobSubIndex reports an .idx file whose .sub file is in the same folder.
func isPairedVobSubIndex(fileName string, fileNames []string) bool {
	stem, isIndex := cutSuffixFold(fileName, vobSubIndexExtension)
	if !isIndex {
		return false
	}

	return slices.ContainsFunc(fileNames, func(other string) bool {
		otherStem, isImage := cutSuffixFold(other, vobSubImageExtension)

		return isImage && otherStem == stem
	})
}

// cutSubtitleFolder returns the file name without its folder. Only the video's own folder
// and a subtitle folder directly below it count; anything deeper belongs to something else.
func cutSubtitleFolder(fileName string) (string, bool) {
	separator := strings.LastIndex(fileName, pathSeparator)
	if separator < 0 {
		return fileName, true
	}

	folder := strings.ToLower(fileName[:separator])

	return fileName[separator+1:], slices.Contains(subtitleFolders(), folder)
}

// cutVideoBase returns the tokens that follow the video base in a subtitle's name without
// extension. The base must end at a token boundary, so "Movie 2.en" is not a subtitle of
// "Movie".
func cutVideoBase(stem, videoBase string) ([]string, bool) {
	rest, hasBase := cutPrefixFold(stem, videoBase)
	if !hasBase {
		return nil, false
	}

	if rest == "" {
		return nil, true
	}

	tokens, atBoundary := strings.CutPrefix(rest, subtitleTokenSeparator)
	if !atBoundary {
		return nil, false
	}

	return strings.Split(tokens, subtitleTokenSeparator), true
}

// cutPrefixFold is strings.CutPrefix under Unicode case folding. It compares rune by rune
// because folding can change the byte length ("K" and the Kelvin sign), which makes cutting
// at len(prefix) wrong.
func cutPrefixFold(text, prefix string) (string, bool) {
	for prefix != "" {
		if text == "" {
			return "", false
		}

		_, wantSize := utf8.DecodeRuneInString(prefix)
		_, gotSize := utf8.DecodeRuneInString(text)

		if !strings.EqualFold(text[:gotSize], prefix[:wantSize]) {
			return "", false
		}

		prefix, text = prefix[wantSize:], text[gotSize:]
	}

	return text, true
}

// cutSuffixFold is strings.CutSuffix for an ASCII suffix in any case.
func cutSuffixFold(text, suffix string) (string, bool) {
	start := len(text) - len(suffix)
	if start < 0 || !strings.EqualFold(text[start:], suffix) {
		return "", false
	}

	return text[:start], true
}

// withNameTokens applies the tokens between video base and extension to the subtitle.
func withNameTokens(subtitle media.ExternalSubtitle, tokens []string) media.ExternalSubtitle {
	languageToken := lastLanguageToken(tokens)

	var title []string

	for position, token := range tokens {
		lower := strings.ToLower(token)

		switch {
		case token == "":
		case position == languageToken:
			subtitle.Language, _ = parseSubtitleLanguage(token)
		case lower == "forced", lower == "foreign":
			subtitle.Forced = true
		case lower == "sdh", lower == "cc", lower == hearingImpairedToken && languageToken != noLanguageToken:
			subtitle.SDH = true
		case lower == "default":
			subtitle.Default = true
		default:
			title = append(title, token)
		}
	}

	subtitle.Title = strings.Join(title, subtitleTitleSeparator)

	return subtitle
}

// lastLanguageToken returns the position of the token that names the language, or
// noLanguageToken. The language sits closest to the extension by convention, so the last
// candidate wins and earlier look-alikes ("it" in "it.follows.en") stay in the title. "hi"
// only names Hindi when no other language is given; otherwise it means hearing impaired.
func lastLanguageToken(tokens []string) int {
	hindi := noLanguageToken

	for position := len(tokens) - 1; position >= 0; position-- {
		if _, isLanguage := parseSubtitleLanguage(tokens[position]); !isLanguage {
			continue
		}

		if !strings.EqualFold(tokens[position], hearingImpairedToken) {
			return position
		}

		if hindi == noLanguageToken {
			hindi = position
		}
	}

	return hindi
}

// parseSubtitleLanguage reads a language token: a listed ISO 639 code, optionally followed
// by a script and a region ("pt-BR", "pt_br", "zh-Hans", "es-419"). The result is written
// the BCP 47 way: language lower case, script capitalised, region upper case.
func parseSubtitleLanguage(token string) (string, bool) {
	subtags := strings.Split(strings.ReplaceAll(token, alternateSubtagSeparator, languageSubtagSeparator), languageSubtagSeparator)
	primary := strings.ToLower(subtags[0])

	if !isLetters(primary, minLanguageLength, maxLanguageLength) ||
		!strings.Contains(subtitleLanguageCodes, " "+primary+" ") {
		return "", false
	}

	language := []string{primary}
	rest := subtags[1:]

	if len(rest) > 0 && isLetters(rest[0], scriptSubtagLength, scriptSubtagLength) {
		language = append(language, strings.ToUpper(rest[0][:1])+strings.ToLower(rest[0][1:]))
		rest = rest[1:]
	}

	if len(rest) > 0 && isRegionSubtag(rest[0]) {
		language = append(language, strings.ToUpper(rest[0]))
		rest = rest[1:]
	}

	return strings.Join(language, languageSubtagSeparator), len(rest) == 0
}

// isRegionSubtag accepts a two-letter country ("BR") or a three-digit UN M.49 area ("419").
func isRegionSubtag(subtag string) bool {
	if isLetters(subtag, regionLength, regionLength) {
		return true
	}

	if len(subtag) != numericRegionLength {
		return false
	}

	for _, char := range subtag {
		if char < '0' || char > '9' {
			return false
		}
	}

	return true
}
