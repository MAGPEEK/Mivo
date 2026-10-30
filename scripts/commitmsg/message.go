package main

import (
	"strings"
	"unicode"
)

const (
	// commentPrefix is git's default core.commentChar. The commit-msg hook sees the message
	// before git strips comments, so the checker strips them itself. It does not read
	// core.commentChar: with another comment character (or "auto"), comment lines count as
	// message text, so a comment directly below the header is rejected as a missing blank
	// line. git's templates put a blank line between the message and the comments.
	commentPrefix = "#"
	// scissorsLine starts the section `git commit --verbose` appends (the staged diff). Git
	// discards it together with everything below.
	scissorsLine = commentPrefix + " ------------------------ >8 ------------------------"
	// byteOrderMark is written by some Windows editors at the start of the message file.
	byteOrderMark = "\uFEFF"
)

// commitMessage is the part of a commit message the checker validates: comments, the
// scissors section and surrounding blank lines are removed. Trailers ("Co-Authored-By: …")
// need no special handling, because only the header and the line below it are checked.
type commitMessage struct {
	header string
	// body holds the lines after the header.
	body []string
}

// parseMessage reduces the raw content of the message file to the message git will store.
func parseMessage(raw string) commitMessage {
	lines := trimBlankLines(meaningfulLines(raw))
	if len(lines) == 0 {
		return commitMessage{}
	}

	return commitMessage{header: lines[0], body: lines[1:]}
}

// meaningfulLines splits the message into lines without line endings, trailing whitespace,
// comment lines and the scissors section.
func meaningfulLines(raw string) []string {
	normalized := strings.ReplaceAll(strings.TrimPrefix(raw, byteOrderMark), "\r\n", "\n")

	var lines []string

	for line := range strings.SplitSeq(normalized, "\n") {
		if line == scissorsLine {
			break
		}

		if strings.HasPrefix(line, commentPrefix) {
			continue
		}

		lines = append(lines, strings.TrimRightFunc(line, unicode.IsSpace))
	}

	return lines
}

// trimBlankLines removes leading and trailing empty lines.
func trimBlankLines(lines []string) []string {
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}

	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return lines
}
