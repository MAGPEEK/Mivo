package main

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	// conventionalTypes are the commit types ADR 0019 allows, in the order the help shows them.
	conventionalTypes = "feat fix docs style refactor perf test build ci chore revert"
	// scopePattern is the optional scope inside the parentheses.
	scopePattern = `[a-z0-9-]+`

	// mergePattern matches the subjects git merge and GitHub generate, e.g.
	// "Merge branch 'main' of https://…" or "Merge pull request #12 from user/branch".
	mergePattern = `^Merge (branch(es)?|remote-tracking branch(es)?|tags?|commits?|pull request) \S`
	// revertPattern matches the subjects of git revert ("Revert "feat: x"") and of reverting
	// a revert ("Reapply "feat: x"").
	revertPattern = `^(Revert|Reapply) ".+"$`
	// autosquashPattern matches git commit --fixup/--squash subjects. They repeat an already
	// checked subject and disappear in git rebase --autosquash.
	autosquashPattern = `^(fixup|squash|amend)! \S`

	formatHelp = `expected format (Conventional Commits, ADR 0019):

  type(scope)!: subject

  type     one of: %s
  (scope)  optional, lower-case letters, digits and "-"
  !        optional, marks a breaking change
  subject  short summary after ": "

examples:
  feat(playback): add HLS seeking
  fix: handle empty library roots
  refactor(sqlite)!: rename the media table

git's own subjects are accepted as they are:
  Merge …, Revert "…", Reapply "…", fixup! …, squash! …, amend! …`
)

var (
	errEmptyMessage     = errors.New("commit message is empty")
	errInvalidHeader    = errors.New("header is not a Conventional Commits header")
	errMissingBlankLine = errors.New("the header must be a single line followed by a blank line")
)

// headerRules holds the compiled header patterns.
type headerRules struct {
	conventional *regexp.Regexp
	gitGenerated []*regexp.Regexp
}

func newHeaderRules() headerRules {
	types := strings.Join(strings.Fields(conventionalTypes), "|")

	return headerRules{
		conventional: regexp.MustCompile(`^(` + types + `)(\(` + scopePattern + `\))?!?: \S`),
		gitGenerated: []*regexp.Regexp{
			regexp.MustCompile(mergePattern),
			regexp.MustCompile(revertPattern),
			regexp.MustCompile(autosquashPattern),
		},
	}
}

// checkMessage validates the raw content of a commit message file. The error explains the
// problem and shows the expected format.
func checkMessage(raw string) error {
	message := parseMessage(raw)
	if message.header == "" {
		return fmt.Errorf("%w\n\n%s", errEmptyMessage, help())
	}

	rules := newHeaderRules()
	if rules.isGitGenerated(message.header) {
		return nil
	}

	if !rules.conventional.MatchString(message.header) {
		return fmt.Errorf("%w: %q\n\n%s", errInvalidHeader, message.header, help())
	}

	if len(message.body) > 0 && message.body[0] != "" {
		return fmt.Errorf("%w: %q continues with %q\n\n%s",
			errMissingBlankLine, message.header, message.body[0], help())
	}

	return nil
}

func (rules headerRules) isGitGenerated(header string) bool {
	for _, pattern := range rules.gitGenerated {
		if pattern.MatchString(header) {
			return true
		}
	}

	return false
}

func help() string {
	return fmt.Sprintf(formatHelp, conventionalTypes)
}
