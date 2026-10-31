package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"unicode/utf8"
)

// leftoverToken matches spkmeta's build-token syntax @NAME@. spkmeta does not scan inserted
// values again, so a value that itself contains a token would survive rendering.
var leftoverToken = regexp.MustCompile(`@[A-Z][A-Z0-9_]*@`)

var (
	// errLeftoverToken reports an @NAME@ token in a staged file.
	errLeftoverToken = errors.New("unreplaced build token")
	// errCarriageReturn reports a CR character, which bash in the chroot and DSM's sh would
	// read as part of a command.
	errCarriageReturn = errors.New("CR line ending")
	// errNonASCII reports a non-ASCII byte in a file the chroot reads with an ASCII locale.
	errNonASCII = errors.New("non-ASCII byte")
	// errInvalidJSON reports a JSON template that no longer parses after rendering.
	errInvalidJSON = errors.New("invalid JSON")
)

// checkEntries applies every content rule to the planned files.
func checkEntries(entries []entry) error {
	for _, item := range entries {
		if err := checkContent(item); err != nil {
			return fmt.Errorf("staged %s: %w", item.path, err)
		}
	}

	return nil
}

func checkContent(item entry) error {
	if item.checks&checkText != 0 {
		if token := leftoverToken.Find(item.content); token != nil {
			return fmt.Errorf("line %d: %s: %w", lineOf(item.content, bytes.Index(item.content, token)), token, errLeftoverToken)
		}

		if index := bytes.IndexByte(item.content, '\r'); index >= 0 {
			return fmt.Errorf("line %d: %w", lineOf(item.content, index), errCarriageReturn)
		}
	}

	if item.checks&checkASCII != 0 {
		for index, character := range item.content {
			if character >= utf8.RuneSelf {
				return fmt.Errorf("line %d: byte 0x%02x: %w", lineOf(item.content, index), character, errNonASCII)
			}
		}
	}

	if item.checks&checkJSON != 0 && !json.Valid(item.content) {
		return errInvalidJSON
	}

	return nil
}

// lineOf returns the 1-based line of a byte offset.
func lineOf(content []byte, offset int) int {
	return bytes.Count(content[:offset], []byte("\n")) + 1
}
