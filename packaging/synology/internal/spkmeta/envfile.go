package spkmeta

import (
	"fmt"
	"regexp"
	"strings"
)

// envNamePattern is the name part of the build.env grammar ^[A-Z_][A-Z0-9_]*='[^']*'$, shared
// by build.env and package.env.
const envNamePattern = `^[A-Z_][A-Z0-9_]*$`

// envAssignment is one line of an env file.
type envAssignment struct {
	name  string
	value string
}

// renderEnv writes one single-quoted sh assignment per line, nothing else: no comments, no
// blank lines, a final newline. Shell scripts source the result; tools may also parse it line
// by line with the grammar, because single quotes need no escaping rules when the value
// contains none.
func renderEnv(assignments []envAssignment) (string, error) {
	namePattern := regexp.MustCompile(envNamePattern)

	var content strings.Builder

	for _, assignment := range assignments {
		if !namePattern.MatchString(assignment.name) {
			return "", fmt.Errorf("env name %q does not match %s: %w", assignment.name, envNamePattern, ErrInvalidValue)
		}

		if err := validateEnvValue(assignment.name, assignment.value); err != nil {
			return "", err
		}

		content.WriteString(assignment.name + "='" + assignment.value + "'\n")
	}

	return content.String(), nil
}
