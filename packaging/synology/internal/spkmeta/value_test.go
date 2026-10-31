package spkmeta_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/packaging/synology/internal/spkmeta"
)

const umlautText = "Medienserver für Geräte im Heimnetz, Größe egal"

// Characters every INFO value must not contain (ADR 0026 Notes "INFO").
func infoUnsafeValues() map[string]string {
	return map[string]string{
		"double quote": `Demo "Server"`,
		"backslash":    `Demo\Server`,
		"dollar":       "Demo $HOME",
		"backtick":     "Demo `id`",
		"tab":          "Demo\tServer",
		"newline":      "Demo\nServer",
	}
}

// Characters the main INFO fields must not contain in addition (pkg_dump_info echoes them
// unquoted).
func infoMainUnsafeValues() map[string]string {
	return map[string]string{
		"asterisk":      "Demo * Server",
		"question mark": "Demo Server?",
		"bracket":       "Demo [Server]",
		"double space":  "Demo  Server",
	}
}

func TestValidateINFOValueRejectsCharactersINFOCannotCarry(t *testing.T) {
	t.Parallel()

	for name, value := range infoUnsafeValues() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if err := spkmeta.ValidateINFOValue("FIELD", value); !errors.Is(err, spkmeta.ErrUnsafeValue) {
				t.Errorf("ValidateINFOValue(%q) = %v, want ErrUnsafeValue", value, err)
			}

			if err := spkmeta.ValidateINFOMainValue("FIELD", value); !errors.Is(err, spkmeta.ErrUnsafeValue) {
				t.Errorf("ValidateINFOMainValue(%q) = %v, want ErrUnsafeValue", value, err)
			}
		})
	}
}

func TestValidateINFOMainValueRejectsGlobCharactersAndDoubleSpaces(t *testing.T) {
	t.Parallel()

	for name, value := range infoMainUnsafeValues() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if err := spkmeta.ValidateINFOMainValue("FIELD", value); !errors.Is(err, spkmeta.ErrUnsafeValue) {
				t.Errorf("ValidateINFOMainValue(%q) = %v, want ErrUnsafeValue", value, err)
			}

			// Values outside the main field loop are echoed quoted, so these pass there.
			if err := spkmeta.ValidateINFOValue("FIELD", value); err != nil {
				t.Errorf("ValidateINFOValue(%q) = %v, want nil", value, err)
			}
		})
	}
}

func TestValidateINFOValueAcceptsUTF8(t *testing.T) {
	t.Parallel()

	for _, validate := range []func(string, string) error{spkmeta.ValidateINFOValue, spkmeta.ValidateINFOMainValue} {
		if err := validate("DESCRIPTION_GER", umlautText); err != nil {
			t.Errorf("umlaut text rejected: %v", err)
		}

		if err := validate("ARCH", "armv7 armada38x monaco"); err != nil {
			t.Errorf("single spaces rejected: %v", err)
		}
	}
}

func TestValidateINFOValueRejectsEmptyAndInvalidUTF8(t *testing.T) {
	t.Parallel()

	if err := spkmeta.ValidateINFOValue("FIELD", ""); !errors.Is(err, spkmeta.ErrInvalidValue) {
		t.Errorf("empty value: %v, want ErrInvalidValue", err)
	}

	if err := spkmeta.ValidateINFOMainValue("FIELD", "Demo \xff"); !errors.Is(err, spkmeta.ErrUnsafeValue) {
		t.Errorf("invalid UTF-8: %v, want ErrUnsafeValue", err)
	}
}

func TestValidateINFOValueNamesTheField(t *testing.T) {
	t.Parallel()

	err := spkmeta.ValidateINFOMainValue("DISPLAY_NAME", "Demo?")
	if err == nil || !containsAll(err.Error(), "DISPLAY_NAME", "question mark") {
		t.Errorf("error %v does not name the field and the character", err)
	}
}
