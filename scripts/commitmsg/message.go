package main

import (
	"strings"
	"unicode"
)

const (
	// commentPrefix is git's default core.commentChar. The commit-msg hook sees the message
	// before git strips comments, so the checker strips them itself.
	commentPrefix = "#"
	// scissorsLine starts the section `git commit --verbose` appends (the staged diff). Git
	// discards it together with everything below.
	scissorsLine = commentPrefix + " ------------------------ >8 ------------------------"
	// byteOrderMark is written by some Windows editors at the start of the message file.
	byteOrderMark = "\uFEFF"
	// trailerSeparator separates a trailer token from its value ("Signed-off-by: A <a@b>").
	trailerSeparator = ":"
)

// commitMessage is the part of a commit message the checker validates: comments, the
// scissors section, surrounding blank lines and the trailer block are removed.
type commitMessage struct {
	header string
	// body holds the lines after the header up to the trailer block.
	body []string
}

// parseMessage reduces the raw content of the message file to the message git will store,
// without its trailer block.
func parseMessage(raw string) commitMessage {
	lines := withoutTrailerBlock(trimBlankLines(meaningfulLines(raw)))
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

// withoutTrailerBlock removes the last paragraph when it consists of git trailers
// ("Co-Authored-By: …"). The first paragraph is never treated as a trailer block, because a
// header such as "chore: subject" has the same shape as a trailer.
func withoutTrailerBlock(lines []string) []string {
	start := lastParagraphStart(lines)
	if start == 0 || !isTrailerBlock(lines[start:]) {
		return lines
	}

	return trimBlankLines(lines[:start])
}

// lastParagraphStart returns the index of the first line after the last blank line, or 0
// when the message has a single paragraph.
func lastParagraphStart(lines []string) int {
	for index := len(lines) - 1; index >= 0; index-- {
		if lines[index] == "" {
			return index + 1
		}
	}

	return 0
}

// isTrailerBlock reports whether the paragraph starts with a trailer and every further line
// is a trailer or an indented continuation of the previous one.
func isTrailerBlock(paragraph []string) bool {
	if len(paragraph) == 0 || !isTrailerLine(paragraph[0]) {
		return false
	}

	for _, line := range paragraph[1:] {
		if !isTrailerLine(line) && !isContinuationLine(line) {
			return false
		}
	}

	return true
}

// isTrailerLine reports whether the line has the form "Token: value", where the token
// consists of ASCII letters, digits and "-" like git's trailer keys.
func isTrailerLine(line string) bool {
	token, value, found := strings.Cut(line, trailerSeparator)
	if !found || token == "" || strings.TrimSpace(value) == "" {
		return false
	}

	for _, character := range token {
		if !isTrailerTokenCharacter(character) {
			return false
		}
	}

	return true
}

func isTrailerTokenCharacter(character rune) bool {
	return character == '-' ||
		(character >= 'a' && character <= 'z') ||
		(character >= 'A' && character <= 'Z') ||
		(character >= '0' && character <= '9')
}

// isContinuationLine reports whether the line continues a folded trailer value.
func isContinuationLine(line string) bool {
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
}
