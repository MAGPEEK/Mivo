package system

import (
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
)

// Layout of the canonical UUID text form: groups of 8-4-4-4-12 hex digits joined by hyphens.
const (
	serverIDLength      = 36
	firstHyphenOffset   = 8
	secondHyphenOffset  = 13
	thirdHyphenOffset   = 18
	fourthHyphenOffset  = 23
	serverIDHyphenGlyph = '-'
)

// The nil and max UUIDs (RFC 9562) are well-formed but carry no randomness. A generator that
// returns one is broken, and persisting it would make every such installation share one ID.
const (
	nilUUID = "00000000-0000-0000-0000-000000000000"
	maxUUID = "ffffffff-ffff-ffff-ffff-ffffffffffff"
)

// ServerID identifies one server installation. It is created once and stays stable, so
// clients (DLNA, webOS app) can recognise the same server across restarts and IP changes.
// The zero value is not a valid ID; obtain one through ParseServerID.
type ServerID struct {
	value string
}

// ParseServerID accepts only the canonical lowercase UUID text form
// (xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx). Uppercase, braces, URN prefixes and the
// hyphen-less form are rejected so that one server never has two textual IDs. The nil and
// max UUIDs are rejected as well. The error wraps domain.ErrInvalid.
func ParseServerID(text string) (ServerID, error) {
	if !isCanonicalUUID(text) {
		return ServerID{}, fmt.Errorf("parse server id %q: not a canonical lowercase UUID: %w",
			text, domain.ErrInvalid)
	}

	if isReservedUUID(text) {
		return ServerID{}, fmt.Errorf("parse server id %q: reserved nil or max UUID: %w",
			text, domain.ErrInvalid)
	}

	return ServerID{value: text}, nil
}

// String returns the canonical lowercase UUID text.
func (id ServerID) String() string {
	return id.value
}

// IsZero reports whether the ID is the zero value, i.e. was not created by ParseServerID.
func (id ServerID) IsZero() bool {
	return id.value == ""
}

func isCanonicalUUID(text string) bool {
	if len(text) != serverIDLength {
		return false
	}

	for position := range len(text) {
		character := text[position]
		if isHyphenOffset(position) {
			if character != serverIDHyphenGlyph {
				return false
			}

			continue
		}

		if !isLowercaseHexDigit(character) {
			return false
		}
	}

	return true
}

func isReservedUUID(text string) bool {
	return text == nilUUID || text == maxUUID
}

func isHyphenOffset(position int) bool {
	switch position {
	case firstHyphenOffset, secondHyphenOffset, thirdHyphenOffset, fourthHyphenOffset:
		return true
	default:
		return false
	}
}

func isLowercaseHexDigit(character byte) bool {
	return ('0' <= character && character <= '9') || ('a' <= character && character <= 'f')
}
