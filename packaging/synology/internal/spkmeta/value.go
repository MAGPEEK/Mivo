package spkmeta

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// forbiddenText is a character (or sequence) a value must not contain, with the words the
// error uses for it.
type forbiddenText struct {
	text        string
	description string
}

// infoForbidden lists what no INFO value may contain: the toolkit sources INFO as shell and
// pkg_dump_info writes key="value" (ADR 0026 Notes "INFO", DSM-NOTES section 2).
func infoForbidden() []forbiddenText {
	return []forbiddenText{
		{text: `"`, description: "a double quote"},
		{text: `\`, description: "a backslash"},
		{text: "$", description: "a dollar sign"},
		{text: "`", description: "a backtick"},
		{text: "\t", description: "a tab"},
		{text: "\n", description: "a newline"},
	}
}

// infoMainForbidden lists what the main INFO fields may not contain in addition:
// pkg_dump_info echoes them unquoted, so glob characters expand against the working directory
// and runs of spaces collapse.
func infoMainForbidden() []forbiddenText {
	return []forbiddenText{
		{text: "*", description: "an asterisk"},
		{text: "?", description: "a question mark"},
		{text: "[", description: "an opening bracket"},
		{text: "  ", description: "two consecutive spaces"},
	}
}

// ValidateINFOValue reports whether value can be written to INFO through build.env: it must
// be non-empty valid UTF-8 without ", \, $, a backtick, a tab or a newline. UTF-8 text such as
// German umlauts is allowed. name is the build.env field, used in the error.
func ValidateINFOValue(name, value string) error {
	if value == "" {
		return fmt.Errorf("%s is empty: %w", name, ErrInvalidValue)
	}

	if !utf8.ValidString(value) {
		return fmt.Errorf("%s %q is not valid UTF-8: %w", name, value, ErrUnsafeValue)
	}

	return rejectForbidden(name, value, infoForbidden())
}

// ValidateINFOMainValue applies ValidateINFOValue and additionally rejects *, ?, [ and two
// consecutive spaces. It is the rule for DISPLAY_NAME, DESCRIPTION, DESCRIPTION_GER,
// MAINTAINER and MAINTAINER_URL.
func ValidateINFOMainValue(name, value string) error {
	if err := ValidateINFOValue(name, value); err != nil {
		return err
	}

	return rejectForbidden(name, value, infoMainForbidden())
}

func rejectForbidden(name, value string, forbidden []forbiddenText) error {
	for _, entry := range forbidden {
		if strings.Contains(value, entry.text) {
			return fmt.Errorf("%s %q contains %s: %w", name, value, entry.description, ErrUnsafeValue)
		}
	}

	return nil
}

// ASCII control characters: everything below the space, and DEL.
const (
	firstPrintableASCII = 0x20
	asciiDelete         = 0x7f
)

// validateEnvValue enforces what a single-quoted sh assignment of the build.env grammar
// ^[A-Z_][A-Z0-9_]*='[^']*'$ can carry: no single quote, and no control character, so every
// assignment stays on its line and the files carry no CR.
func validateEnvValue(name, value string) error {
	if strings.Contains(value, "'") {
		return fmt.Errorf("%s %q contains a single quote: %w", name, value, ErrUnsafeValue)
	}

	for index := range len(value) {
		if character := value[index]; character < firstPrintableASCII || character == asciiDelete {
			return fmt.Errorf("%s %q contains the control character 0x%02x: %w", name, value, character, ErrUnsafeValue)
		}
	}

	if !utf8.ValidString(value) {
		return fmt.Errorf("%s %q is not valid UTF-8: %w", name, value, ErrUnsafeValue)
	}

	return nil
}
