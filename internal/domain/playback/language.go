package playback

import "strings"

// undeterminedLanguage is the ISO 639-2 code for "no language given"; ffprobe and Matroska
// write it for untagged tracks.
const undeterminedLanguage = "und"

// Bounds of a preferred language tag such as "de", "deu" or "pt-BR".
const (
	maxLanguageTagLength   = 16
	minPrimarySubtagLength = 2
	maxPrimarySubtagLength = 3
)

// languageCodes lists languages as "<ISO 639-1> [<ISO 639-2/B>] <ISO 639-2/T>" entries,
// separated by ";". The last code of an entry is the normal form. Some entries also carry
// deprecated ISO 639-1 codes (iw, in, ji), and Norwegian Bokmål and Nynorsk share the
// "nor" form, because files tag Norwegian as any of them. It is a constant instead
// of a map so the package keeps no global state. Languages missing here still match when
// both sides use the same code.
const languageCodes = "af afr;sq alb sqi;am amh;ar ara;hy arm hye;az aze;eu baq eus;be bel;" +
	"bn ben;bs bos;br bre;bg bul;my bur mya;ca cat;zh chi zho;hr hrv;cs cze ces;da dan;" +
	"nl dut nld;en eng;eo epo;et est;fo fao;fi fin;fr fre fra;gl glg;ka geo kat;de ger deu;" +
	"el gre ell;gu guj;ha hau;he iw heb;hi hin;hu hun;is ice isl;ig ibo;id in ind;ga gle;gd gla;" +
	"it ita;ja jpn;kn kan;kk kaz;km khm;ko kor;ku kur;ky kir;lo lao;la lat;lv lav;lt lit;" +
	"lb ltz;mk mac mkd;mg mlg;ms may msa;ml mal;mt mlt;mi mao mri;mr mar;mn mon;ne nep;" +
	"nb nn no nob nno nor;oc oci;ps pus;fa per fas;pl pol;pt por;pa pan;ro rum ron;ru rus;" +
	"sr srp;si sin;sk slo slk;sl slv;so som;es spa;sw swa;sv swe;tl tgl;tg tgk;ta tam;" +
	"te tel;th tha;bo tib bod;tr tur;tk tuk;uk ukr;ur urd;uz uzb;vi vie;cy wel cym;xh xho;" +
	"yi ji yid;yo yor;zu zul"

// normalizeLanguage returns the ISO 639-2/T form of a language tag, so "de", "ger", "deu"
// and "de-AT" compare equal. Only the primary subtag counts. Empty and "und" return "";
// unknown codes return their lower-case primary subtag.
func normalizeLanguage(tag string) string {
	primary, _, _ := strings.Cut(unifySeparators(strings.TrimSpace(tag)), "-")
	if primary == "" || primary == undeterminedLanguage {
		return ""
	}

	for entry := range strings.SplitSeq(languageCodes, ";") {
		codes := strings.Fields(entry)
		for _, code := range codes {
			if code == primary {
				return codes[len(codes)-1]
			}
		}
	}

	return primary
}

// languageRank is the position of the language in the preferred list, or the list length
// when it is not listed or undetermined. Undetermined entries of the list never match.
func languageRank(language string, preferred []string) int {
	normal := normalizeLanguage(language)
	if normal == "" {
		return len(preferred)
	}

	for position, candidate := range preferred {
		if normalizeLanguage(candidate) == normal {
			return position
		}
	}

	return len(preferred)
}

// isLanguageTag accepts what a client may send as a preferred language: a primary subtag
// of two or three letters, optionally followed by subtags of letters and digits ("pt-BR").
func isLanguageTag(tag string) bool {
	if len(tag) > maxLanguageTagLength {
		return false
	}

	for index, subtag := range strings.Split(unifySeparators(tag), "-") {
		if !isSubtag(subtag, index == 0) {
			return false
		}
	}

	return true
}

// unifySeparators lower-cases a tag and writes "pt_BR" as "pt-br".
func unifySeparators(tag string) string {
	return strings.ReplaceAll(strings.ToLower(tag), "_", "-")
}

// isSubtag checks one lower-case subtag; the primary one has two or three letters only.
func isSubtag(subtag string, primary bool) bool {
	if subtag == "" ||
		(primary && (len(subtag) < minPrimarySubtagLength || len(subtag) > maxPrimarySubtagLength)) {
		return false
	}

	for _, char := range subtag {
		letter := char >= 'a' && char <= 'z'
		digit := char >= '0' && char <= '9'

		if !letter && (primary || !digit) {
			return false
		}
	}

	return true
}
