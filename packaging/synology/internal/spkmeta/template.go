package spkmeta

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// placeholderPattern matches a build-time token such as @DISPLAY_NAME@. {{...}} belongs to
// DSM's wizard and resource templating and ${...} to sh, so neither is touched.
const placeholderPattern = `@[A-Z][A-Z0-9_]*@`

// asciiOnlyDirectory holds the toolkit files the chroot's Python 3.4 reads with an ASCII
// locale; one non-ASCII byte silently empties the dependency list (ADR 0026 Notes).
const asciiOnlyDirectory = "SynoBuildConf/"

// RenderTemplate fills the build-time tokens @ID@, @DISPLAY_NAME@, @ENV_PREFIX@, @PORT@,
// @VERSION@ and @DSM_VERSION@ of a template under packaging/synology/package/. name is the
// template's path relative to that directory (slash or backslash separated); it appears in
// errors and decides the ASCII rule.
//
// Inserted values are not scanned again. An @UPPER_CASE@ token without a value is an error
// naming the template and line, so no token reaches the SPK. A file under SynoBuildConf/ must
// render to pure ASCII.
func (build Build) RenderTemplate(name, content string) (string, error) {
	if err := build.Validate(); err != nil {
		return "", fmt.Errorf("render %s: %w", name, err)
	}

	values := build.templateValues()
	tokenPattern := regexp.MustCompile(placeholderPattern)
	lines := strings.Split(content, "\n")

	for index, line := range lines {
		rendered, err := replaceTokens(line, tokenPattern, values)
		if err != nil {
			return "", fmt.Errorf("render %s line %d: %w", name, index+1, err)
		}

		lines[index] = rendered
	}

	output := strings.Join(lines, "\n")

	if requiresASCII(name) {
		if err := requireASCII(output); err != nil {
			return "", fmt.Errorf("render %s %w", name, err)
		}
	}

	return output, nil
}

func replaceTokens(line string, tokenPattern *regexp.Regexp, values map[string]string) (string, error) {
	unknown := ""

	rendered := tokenPattern.ReplaceAllStringFunc(line, func(token string) string {
		value, known := values[strings.Trim(token, "@")]
		if !known {
			if unknown == "" {
				unknown = token
			}

			return token
		}

		return value
	})

	if unknown != "" {
		return "", fmt.Errorf("token %s: %w", unknown, ErrUnknownToken)
	}

	return rendered, nil
}

func requiresASCII(name string) bool {
	normalized := strings.TrimPrefix(strings.ReplaceAll(name, `\`, "/"), "./")

	return strings.HasPrefix(normalized, asciiOnlyDirectory)
}

// requireASCII names the line of the first non-ASCII byte.
func requireASCII(text string) error {
	for index := range len(text) {
		if text[index] >= utf8.RuneSelf {
			line := strings.Count(text[:index], "\n") + 1

			return fmt.Errorf("line %d: byte 0x%02x: %w", line, text[index], ErrNonASCII)
		}
	}

	return nil
}
