package account

import (
	"fmt"
	"unicode/utf8"
)

// Password length limits in bytes of UTF-8 (ADR 0032 §1). The upper bound keeps the argon2id
// input bounded; there are no composition rules.
const (
	MinPasswordBytes = 8
	MaxPasswordBytes = 128
)

// PINLength is the number of ASCII digits of a profile PIN (ADR 0032 §2, US-02).
const PINLength = 4

// ValidatePassword checks the shape of a new password: valid UTF-8 and 8–128 bytes. The
// password is used verbatim (no trimming), so what the user typed is what gets hashed. The
// error is an *InvalidFieldError for FieldPassword.
func ValidatePassword(password string) error {
	if !utf8.ValidString(password) {
		return invalidField(FieldPassword, "must be valid UTF-8")
	}

	if len(password) < MinPasswordBytes || len(password) > MaxPasswordBytes {
		return invalidField(FieldPassword,
			fmt.Sprintf("must have %d to %d bytes", MinPasswordBytes, MaxPasswordBytes))
	}

	return nil
}

// ValidatePIN checks that pin is exactly four ASCII digits. Other Unicode digits (Arabic-Indic,
// fullwidth) are refused so a PIN pad on any device can always type it. The error is an
// *InvalidFieldError for FieldPIN.
func ValidatePIN(pin string) error {
	if len(pin) != PINLength {
		return invalidPIN()
	}

	for index := range len(pin) {
		if pin[index] < '0' || pin[index] > '9' {
			return invalidPIN()
		}
	}

	return nil
}

func invalidPIN() error {
	return invalidField(FieldPIN, fmt.Sprintf("must be exactly %d ASCII digits", PINLength))
}
