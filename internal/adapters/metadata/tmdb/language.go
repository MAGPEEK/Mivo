package tmdb

import "strings"

const (
	// fallbackLanguage fills fields TMDB has no translation for, and replaces a missing or
	// malformed language tag.
	fallbackLanguage = "en-US"
	englishPrimary   = "en"
	// fallbackRegion picks the content rating when the language names no region TMDB knows.
	fallbackRegion = "US"

	maxLanguageTagLength = 12
	languageSubtagLength = 2
)

// primaryRegion returns the usual country of languages whose country code differs from the
// language code, else the upper-cased language code.
func primaryRegion(primary string) string {
	switch primary {
	case "en":
		return "US"
	case "ja":
		return "JP"
	case "ko":
		return "KR"
	case "zh":
		return "CN"
	case "sv":
		return "SE"
	case "da":
		return "DK"
	case "cs":
		return "CZ"
	case "el":
		return "GR"
	case "uk":
		return "UA"
	case "he":
		return "IL"
	case "hi":
		return "IN"
	case "nb":
		return "NO"
	default:
		return strings.ToUpper(primary)
	}
}

// language is a validated metadata language: the tag sent to TMDB, its primary subtag and
// the region whose content rating is used.
type language struct {
	tag     string
	primary string
	region  string
}

// parseLanguage accepts "de", "pt-BR" or "pt_BR"; anything else selects English.
func parseLanguage(text string) language {
	tag := strings.ReplaceAll(strings.TrimSpace(text), "_", "-")
	if !isLanguageTag(tag) {
		tag = fallbackLanguage
	}

	primary, region, hasRegion := strings.Cut(tag, "-")
	primary = strings.ToLower(primary)

	switch {
	case hasRegion && len(region) == languageSubtagLength:
		region = strings.ToUpper(region)
		tag = primary + "-" + region
	default:
		region, tag = primaryRegion(primary), primary
	}

	return language{tag: tag, primary: primary, region: region}
}

func isLanguageTag(tag string) bool {
	if len(tag) < languageSubtagLength || len(tag) > maxLanguageTagLength {
		return false
	}

	primary, _, _ := strings.Cut(tag, "-")
	if len(primary) != languageSubtagLength {
		return false
	}

	for _, character := range tag {
		if !isASCIILetter(character) && character != '-' {
			return false
		}
	}

	return true
}

func isASCIILetter(character rune) bool {
	return (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z')
}

// isEnglish reports whether no English fallback request is needed.
func (lang language) isEnglish() bool {
	return lang.primary == englishPrimary
}

// imageLanguages lists the picture languages to include: the metadata language, pictures
// without text, then English.
func (lang language) imageLanguages() string {
	if lang.isEnglish() {
		return englishPrimary + ",null"
	}

	return lang.primary + ",null," + englishPrimary
}
