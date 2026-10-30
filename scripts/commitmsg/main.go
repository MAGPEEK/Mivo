// Command commitmsg checks that a commit message follows Conventional Commits (ADR 0019).
//
// lefthook runs it as the commit-msg hook with the path of the message file git wrote:
//
//	go run ./scripts/commitmsg .git/COMMIT_EDITMSG
//
// It exits with 0 for a valid message, 1 for an invalid or unreadable message and 2 for a
// wrong command line. The reason and the expected format go to stderr.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	exitOK = 0
	// exitFailure rejects the commit: the message is invalid or cannot be read.
	exitFailure = 1
	// exitUsage matches the exit code of the flag package for invalid command lines.
	exitUsage = 2

	usageText = "usage: commitmsg <commit message file>"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

// run checks the message file named by the only argument and returns the process exit code.
func run(args []string, stderr io.Writer) int {
	if len(args) != 1 {
		// A failed write to stderr cannot be reported anywhere else.
		_, _ = fmt.Fprintln(stderr, usageText)

		return exitUsage
	}

	// The path comes from git: it names the message file in the git directory, which lies
	// outside the working tree for linked worktrees, so it cannot be confined to a root.
	content, err := os.ReadFile(filepath.Clean(args[0])) //nolint:gosec // developer tool reading the file git passes to the hook
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "commitmsg: read message file: %v\n", err)

		return exitFailure
	}

	if err := checkMessage(string(content)); err != nil {
		_, _ = fmt.Fprintf(stderr, "commitmsg: %v\n", err)

		return exitFailure
	}

	return exitOK
}
