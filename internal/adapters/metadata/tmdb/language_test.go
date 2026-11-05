package tmdb

import "testing"

func TestParseLanguage(t *testing.T) {
	tests := []struct {
		text string
		want language
	}{
		{"de", language{tag: "de", primary: "de", region: "DE"}},
		{"DE", language{tag: "de", primary: "de", region: "DE"}},
		{"pt-BR", language{tag: "pt-BR", primary: "pt", region: "BR"}},
		{"pt_br", language{tag: "pt-BR", primary: "pt", region: "BR"}},
		{"en", language{tag: "en", primary: "en", region: "US"}},
		{"ja", language{tag: "ja", primary: "ja", region: "JP"}},
		{"zh-Hant", language{tag: "zh", primary: "zh", region: "CN"}},
		{"", language{tag: "en-US", primary: "en", region: "US"}},
		{"deu", language{tag: "en-US", primary: "en", region: "US"}},
		{"d1", language{tag: "en-US", primary: "en", region: "US"}},
		{"de&x=1", language{tag: "en-US", primary: "en", region: "US"}},
		{"de-DE-extra-long-tag", language{tag: "en-US", primary: "en", region: "US"}},
	}

	for _, test := range tests {
		if got := parseLanguage(test.text); got != test.want {
			t.Errorf("parseLanguage(%q) = %+v, want %+v", test.text, got, test.want)
		}
	}
}

func TestPrimaryRegion(t *testing.T) {
	tests := map[string]string{
		"en": "US", "ja": "JP", "ko": "KR", "zh": "CN", "sv": "SE", "da": "DK", "cs": "CZ",
		"el": "GR", "uk": "UA", "he": "IL", "hi": "IN", "nb": "NO", "fr": "FR",
	}

	for primary, want := range tests {
		if got := primaryRegion(primary); got != want {
			t.Errorf("primaryRegion(%q) = %q, want %q", primary, got, want)
		}
	}
}
