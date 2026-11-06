package account_test

import (
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/domain/account"
)

func TestValidatePassword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{name: "7 bytes", password: strings.Repeat("a", 7), wantErr: true},
		{name: "8 bytes", password: strings.Repeat("a", 8)},
		{name: "128 bytes", password: strings.Repeat("a", 128)},
		{name: "129 bytes", password: strings.Repeat("a", 129), wantErr: true},
		{name: "empty", password: "", wantErr: true},
		{name: "counts bytes not runes: 4 runes are 8 bytes", password: strings.Repeat("ü", 4)},
		{name: "counts bytes not runes: 65 runes are 130 bytes", password: strings.Repeat("ü", 65), wantErr: true},
		{name: "spaces are kept and count", password: "        "},
		{name: "invalid UTF-8", password: "secret\xff\xfe", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := account.ValidatePassword(test.password)
			if test.wantErr {
				assertInvalidField(t, err, account.FieldPassword)

				return
			}

			if err != nil {
				t.Errorf("ValidatePassword(%d bytes) error = %v", len(test.password), err)
			}
		})
	}
}

func TestValidatePIN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pin     string
		wantErr bool
	}{
		{name: "all zeros", pin: "0000"},
		{name: "digits", pin: "1234"},
		{name: "nines", pin: "9999"},
		{name: "three digits", pin: "123", wantErr: true},
		{name: "five digits", pin: "12345", wantErr: true},
		{name: "empty", pin: "", wantErr: true},
		{name: "letter", pin: "12a4", wantErr: true},
		{name: "space", pin: "12 4", wantErr: true},
		{name: "Arabic-Indic digits", pin: "١٢٣٤", wantErr: true},
		{name: "fullwidth digits", pin: "１２３４", wantErr: true},
		{name: "sign", pin: "+123", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := account.ValidatePIN(test.pin)
			if test.wantErr {
				assertInvalidField(t, err, account.FieldPIN)

				return
			}

			if err != nil {
				t.Errorf("ValidatePIN(%q) error = %v", test.pin, err)
			}
		})
	}
}
