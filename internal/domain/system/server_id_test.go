package system_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/system"
)

const canonicalServerID = "f81d4fae-7dec-11d0-a765-00a0c91e6bf6"

func TestParseServerIDAcceptsCanonicalLowercaseUUIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
	}{
		{name: "version 4", text: "0b8c6d4e-3f2a-4c1b-9d8e-7f6a5b4c3d2e"},
		{name: "version 1", text: canonicalServerID},
		{name: "one above nil uuid", text: "00000000-0000-0000-0000-000000000001"},
		{name: "one below max uuid", text: "ffffffff-ffff-ffff-ffff-fffffffffffe"},
		{name: "all hex letters", text: "abcdefab-cdef-abcd-efab-cdefabcdefab"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			id, err := system.ParseServerID(test.text)
			if err != nil {
				t.Fatalf("ParseServerID(%q) error = %v, want nil", test.text, err)
			}

			if got := id.String(); got != test.text {
				t.Errorf("String() = %q, want %q", got, test.text)
			}

			if id.IsZero() {
				t.Error("IsZero() = true for a parsed ID, want false")
			}
		})
	}
}

func TestParseServerIDRejectsNonCanonicalText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
	}{
		{name: "empty", text: ""},
		{name: "uppercase", text: "F81D4FAE-7DEC-11D0-A765-00A0C91E6BF6"},
		{name: "mixed case", text: "f81d4fae-7dec-11d0-A765-00a0c91e6bf6"},
		{name: "braces", text: "{f81d4fae-7dec-11d0-a765-00a0c91e6bf6}"},
		{name: "urn prefix", text: "urn:uuid:f81d4fae-7dec-11d0-a765-00a0c91e6bf6"},
		{name: "without hyphens", text: "f81d4fae7dec11d0a76500a0c91e6bf6"},
		{name: "too short", text: "f81d4fae-7dec-11d0-a765-00a0c91e6bf"},
		{name: "too long", text: "f81d4fae-7dec-11d0-a765-00a0c91e6bf6a"},
		{name: "non hex letter", text: "g81d4fae-7dec-11d0-a765-00a0c91e6bf6"},
		{name: "hyphen moved", text: "f81d4fa-e7dec-11d0-a765-00a0c91e6bf6"},
		{name: "underscore separator", text: "f81d4fae_7dec_11d0_a765_00a0c91e6bf6"},
		{name: "surrounding space", text: " f81d4fae-7dec-11d0-a765-00a0c91e6bf"},
		{name: "multibyte character", text: "f81d4fae-7dec-11d0-a765-00a0c91e6bä"},
		{name: "nil uuid", text: "00000000-0000-0000-0000-000000000000"},
		{name: "max uuid", text: "ffffffff-ffff-ffff-ffff-ffffffffffff"},
		{name: "uppercase max uuid", text: "FFFFFFFF-FFFF-FFFF-FFFF-FFFFFFFFFFFF"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			id, err := system.ParseServerID(test.text)
			if !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("ParseServerID(%q) error = %v, want ErrInvalid", test.text, err)
			}

			if !id.IsZero() {
				t.Errorf("ParseServerID(%q) = %q on error, want zero ID", test.text, id)
			}
		})
	}
}

func TestZeroServerID(t *testing.T) {
	t.Parallel()

	var id system.ServerID
	if !id.IsZero() {
		t.Error("IsZero() = false for the zero value, want true")
	}

	if got := id.String(); got != "" {
		t.Errorf("String() = %q for the zero value, want empty", got)
	}
}

func TestServerIDsCompareByValue(t *testing.T) {
	t.Parallel()

	first, firstErr := system.ParseServerID(canonicalServerID)
	second, secondErr := system.ParseServerID(canonicalServerID)

	if err := errors.Join(firstErr, secondErr); err != nil {
		t.Fatalf("ParseServerID error = %v", err)
	}

	if first != second {
		t.Errorf("IDs parsed from the same text differ: %q != %q", first, second)
	}
}
