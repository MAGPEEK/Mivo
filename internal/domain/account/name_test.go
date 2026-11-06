package account_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/account"
)

// Escaped runes used by the tests; source files stay free of invisible characters.
const (
	composedEAcute    = "\U000000E9" // e with acute, NFC
	decomposedEAcute  = "e\U00000301"
	composedUUmlaut   = "\U000000FC"
	decomposedUUmlaut = "u\U00000308"
	dottedCapitalI    = "\U00000130"
	decomposedDottedI = "I\U00000307"
)

// composeLatin stands in for golang.org/x/text/unicode/norm.NFC.String, which the
// standard-library-only domain cannot import: it composes the decomposed sequences these
// tests use, as the adapter boundary does for every input.
func composeLatin(text string) string {
	return strings.NewReplacer(
		decomposedEAcute, composedEAcute,
		decomposedUUmlaut, composedUUmlaut,
		decomposedDottedI, dottedCapitalI,
	).Replace(text)
}

func TestNormalizeName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "plain", raw: "Anna", want: "Anna"},
		{name: "trims surrounding white space", raw: " \t Anna Lena \n", want: "Anna Lena"},
		{name: "composes NFD to NFC", raw: "Ren" + decomposedEAcute, want: "Ren" + composedEAcute},
		{name: "keeps NFC unchanged", raw: "Ren" + composedEAcute, want: "Ren" + composedEAcute},
		{name: "collapses inner spaces", raw: "Anna   Lena", want: "Anna Lena"},
		{name: "collapses a no-break space", raw: "Anna\U000000A0Lena", want: "Anna Lena"},
		{name: "collapses mixed space separators", raw: "Anna \U00003000\U00002009 Lena", want: "Anna Lena"},
		{name: "trims a leading no-break space", raw: "\U000000A0Anna", want: "Anna"},
		{name: "number only", raw: "42", want: "42"},
		{name: "Chinese name", raw: "\U0000674E\U0000534E", want: "\U0000674E\U0000534E"},
		{name: "Arabic name", raw: "\U00000645\U0000062D\U00000645\U0000062F", want: "\U00000645\U0000062D\U00000645\U0000062F"},
		{name: "letter with combining mark", raw: "a\U00000301", want: "a\U00000301"},
		{name: "one rune", raw: "A", want: "A"},
		{name: "forty runes", raw: strings.Repeat(composedUUmlaut, account.MaxNameRunes), want: strings.Repeat(composedUUmlaut, account.MaxNameRunes)},
		{name: "forty runes after composition", raw: strings.Repeat(decomposedUUmlaut, account.MaxNameRunes), want: strings.Repeat(composedUUmlaut, account.MaxNameRunes)},
		{name: "forty runes after collapsing", raw: strings.Repeat("a", 20) + "     " + strings.Repeat("b", 19), want: strings.Repeat("a", 20) + " " + strings.Repeat("b", 19)},
		{name: "forty-one runes", raw: strings.Repeat("a", account.MaxNameRunes+1), wantErr: true},
		{name: "empty", raw: "", wantErr: true},
		{name: "only white space", raw: "  \t ", wantErr: true},
		{name: "control character", raw: "An\x00na", wantErr: true},
		{name: "inner newline", raw: "An\nna", wantErr: true},
		{name: "inner tab", raw: "An\tna", wantErr: true},
		{name: "delete character", raw: "Anna\x7f", wantErr: true},
		{name: "line separator", raw: "An\U00002028na", wantErr: true},
		{name: "paragraph separator", raw: "An\U00002029na", wantErr: true},
		{name: "right-to-left override", raw: "Anna\U0000202Etxt.exe", wantErr: true},
		{name: "bidi isolate", raw: "\U00002066Anna\U00002069", wantErr: true},
		{name: "zero-width space inside", raw: "An\U0000200Bna", wantErr: true},
		{name: "only zero-width spaces", raw: "\U0000200B\U0000200B", wantErr: true},
		{name: "word joiner", raw: "An\U00002060na", wantErr: true},
		{name: "byte order mark", raw: "\U0000FEFFAnna", wantErr: true},
		{name: "soft hyphen", raw: "An\U000000ADna", wantErr: true},
		{name: "zero-width joiner inside", raw: "An\U0000200Dna", wantErr: true},
		{name: "zero-width joiner in an emoji", raw: "Dev \U0001F469\U0000200D\U0001F4BB", wantErr: true},
		{name: "zero-width non-joiner", raw: "\U00000645\U000006CC\U0000200C\U00000634\U00000648\U00000645", wantErr: true},
		{name: "only zero-width joiners", raw: "\U0000200D\U0000200D", wantErr: true},
		{name: "leading zero-width non-joiner", raw: "\U0000200CAnna", wantErr: true},
		{name: "only combining marks", raw: "\U00000301\U00000308", wantErr: true},
		{name: "only punctuation and symbols", raw: "-- \U0001F600 --", wantErr: true},
		{name: "Hangul filler", raw: "\U00003164", wantErr: true},
		{name: "Hangul filler inside", raw: "An\U00003164na", wantErr: true},
		{name: "Hangul choseong filler", raw: "\U0000115F", wantErr: true},
		{name: "Hangul jungseong filler", raw: "\U00001160", wantErr: true},
		{name: "halfwidth Hangul filler", raw: "\U0000FFA0", wantErr: true},
		{name: "braille blank", raw: "\U00002800\U00002800", wantErr: true},
		{name: "emoji variation selector VS16", raw: "Anna\U0000FE0F", wantErr: true},
		{name: "ideographic variation selector", raw: "Anna\U000E0100", wantErr: true},
		{name: "combining grapheme joiner", raw: "An\U0000034Fna", wantErr: true},
		{name: "Mongolian free variation selector", raw: "An\U0000180Bna", wantErr: true},
		{name: "Khmer inherent vowel", raw: "An\U000017B4na", wantErr: true},
		{name: "unassigned default-ignorable", raw: "An\U00002065na", wantErr: true},
		{name: "invalid UTF-8", raw: "An\xffna", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := account.NormalizeName(test.raw, composeLatin)
			if test.wantErr {
				assertInvalidField(t, err, account.FieldName)

				return
			}

			if err != nil {
				t.Fatalf("NormalizeName(%q) error = %v", test.raw, err)
			}

			if got != test.want {
				t.Errorf("NormalizeName(%q) = %q, want %q", test.raw, got, test.want)
			}
		})
	}
}

func TestNormalizeNameRequiresComposer(t *testing.T) {
	t.Parallel()

	_, err := account.NormalizeName("Anna", nil)
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("NormalizeName(nil composer) error = %v, want domain.ErrInvalid", err)
	}

	var fieldError *account.InvalidFieldError
	if errors.As(err, &fieldError) {
		t.Errorf("NormalizeName(nil composer) error = %v, want a wiring error, not a field error", err)
	}
}

func TestNameKeyFoldsCase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		left  string
		right string
		same  bool
	}{
		{name: "ASCII case", left: "Anna", right: "aNNA", same: true},
		{name: "umlaut case", left: "\U000000DCmit", right: composedUUmlaut + "MIT", same: true},
		{name: "capital sharp s folds to sharp s", left: "STRA\U00001E9EE", right: "stra\U000000DFe", same: true},
		{name: "sharp s is not ss", left: "Stra\U000000DFe", right: "Strasse", same: false},
		{name: "dotted capital I folds to i", left: dottedCapitalI + "rem", right: "irem", same: true},
		{name: "dotless i folds to i", left: "\U00000131rem", right: "IREM", same: true},
		{name: "final sigma", left: "\U0000039F\U000003A3", right: "\U000003BF\U000003C2", same: true},
		{name: "kelvin sign", left: "\U0000212A", right: "k", same: true},
		{name: "different names", left: "Anna", right: "Anne", same: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			left, right := account.NameKey(test.left), account.NameKey(test.right)
			if (left == right) != test.same {
				t.Errorf("NameKey(%q) = %q, NameKey(%q) = %q, want same = %v",
					test.left, left, test.right, right, test.same)
			}
		})
	}
}

func TestNFDAndNFCNamesShareOneKey(t *testing.T) {
	t.Parallel()

	pairs := [][2]string{
		{"Ren" + decomposedEAcute, "REN\U000000C9"},
		{decomposedDottedI + "rem", "irem"},
	}

	for _, pair := range pairs {
		decomposed, err := account.NormalizeName(pair[0], composeLatin)
		if err != nil {
			t.Fatalf("NormalizeName(%q) error = %v", pair[0], err)
		}

		composed, err := account.NormalizeName(pair[1], composeLatin)
		if err != nil {
			t.Fatalf("NormalizeName(%q) error = %v", pair[1], err)
		}

		if account.NameKey(decomposed) != account.NameKey(composed) {
			t.Errorf("NameKey differs for %q and %q", pair[0], pair[1])
		}
	}
}

func assertInvalidField(t *testing.T, err error, field string) {
	t.Helper()

	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("error = %v, want domain.ErrInvalid", err)
	}

	var fieldError *account.InvalidFieldError
	if !errors.As(err, &fieldError) {
		t.Fatalf("error = %v, want *account.InvalidFieldError", err)
	}

	if fieldError.Field != field {
		t.Errorf("field = %q, want %q", fieldError.Field, field)
	}
}
