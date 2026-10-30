package main

import (
	"errors"
	"strings"
	"testing"
)

func TestCheckMessageAcceptsValidMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message string
	}{
		{name: "type and subject", message: "chore: hook test"},
		{name: "scope", message: "feat(playback): add HLS seeking"},
		{name: "scope with digits and dash", message: "fix(m1-21): handle empty roots"},
		{name: "breaking change", message: "refactor!: drop the legacy API"},
		{name: "breaking change with scope", message: "refactor(sqlite)!: rename the media table"},
		{name: "every type docs", message: "docs: explain hooks"},
		{name: "every type style", message: "style: reformat"},
		{name: "every type perf", message: "perf: cache probes"},
		{name: "every type test", message: "test: cover trailers"},
		{name: "every type build", message: "build(go): pin lefthook"},
		{name: "every type ci", message: "ci: add workflow"},
		{name: "every type revert", message: "revert: undo the probe cache"},
		{name: "trailing newline from git", message: "feat: add x\n"},
		{name: "body after blank line", message: "feat: add x\n\nWhy it matters.\nMore detail.\n"},
		{
			name:    "trailer block",
			message: "feat: add x\n\nCo-Authored-By: Claude <noreply@anthropic.com>\n",
		},
		{
			name:    "header, body and trailers",
			message: "fix: y\n\nBody.\n\nSigned-off-by: A <a@example.com>\nCo-Authored-By: B <b@example.com>\n",
		},
		{name: "windows line endings", message: "feat: add x\r\n\r\nBody.\r\n"},
		{name: "byte order mark", message: "\uFEFFfeat: add x\n"},
		{name: "leading blank lines", message: "\n\nfeat: add x\n"},
		{
			name:    "comment lines from the editor template",
			message: "# Please enter the commit message\nfeat: add x\n# Lines starting with '#' are ignored.\n",
		},
		{
			name: "verbose diff below scissors",
			message: "feat: add x\n\n" + scissorsLine + "\n# Do not modify the line above.\n" +
				"diff --git a/x b/x\n+added line\n",
		},
		{name: "merge branch", message: "Merge branch 'feature/x'\n"},
		{name: "merge branch of remote", message: "Merge branch 'main' of https://example.com/mivo\n"},
		{name: "merge remote-tracking branch", message: "Merge remote-tracking branch 'origin/main'\n"},
		{name: "merge branches", message: "Merge branches 'a' and 'b'\n"},
		{name: "merge tag", message: "Merge tag 'v0.1.0'\n"},
		{name: "merge commit", message: "Merge commit 'abc123'\n"},
		{name: "merge pull request", message: "Merge pull request #12 from user/branch\n"},
		{name: "merge branch into", message: "Merge branch 'feature/x' into main\n"},
		{
			name:    "merge remote-tracking branch into",
			message: "Merge remote-tracking branch 'origin/main' into feature/x\n",
		},
		{
			name:    "merge with conflict comments",
			message: "Merge branch 'x'\n\n# Conflicts:\n#\tgo.mod\n",
		},
		{
			name:    "revert",
			message: "Revert \"feat: add x\"\n\nThis reverts commit 0123456789abcdef.\n",
		},
		{name: "reapply", message: "Reapply \"feat: add x\"\n"},
		{
			name:    "revert of a revert",
			message: "Revert \"Revert \"feat: add x\"\"\n\nThis reverts commit 0123456789abcdef.\n",
		},
		{name: "reapply of a nested revert", message: "Reapply \"Revert \"feat: add x\"\"\n"},
		{name: "fixup", message: "fixup! feat: add x\n"},
		{name: "squash", message: "squash! feat: add x\n"},
		{name: "amend", message: "amend! feat: add x\n\nfeat: add x properly\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if err := checkMessage(test.message); err != nil {
				t.Fatalf("checkMessage(%q) = %v, want nil", test.message, err)
			}
		})
	}
}

func TestCheckMessageRejectsInvalidMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message string
		want    error
	}{
		{name: "empty", message: "", want: errEmptyMessage},
		{name: "only whitespace", message: "\n  \n\t\n", want: errEmptyMessage},
		{name: "only comments", message: "# Please enter the commit message\n#\n", want: errEmptyMessage},
		{name: "only diff below scissors", message: scissorsLine + "\ndiff --git a/x b/x\n", want: errEmptyMessage},
		{name: "no type", message: "update stuff", want: errInvalidHeader},
		{name: "unknown type", message: "feature: add x", want: errInvalidHeader},
		{name: "capitalised type", message: "Feat: add x", want: errInvalidHeader},
		{name: "missing space after colon", message: "feat:add x", want: errInvalidHeader},
		{name: "empty subject", message: "feat: ", want: errInvalidHeader},
		{name: "subject starts with second space", message: "feat:  add x", want: errInvalidHeader},
		{name: "empty scope", message: "feat(): add x", want: errInvalidHeader},
		{name: "upper-case scope", message: "feat(Playback): add x", want: errInvalidHeader},
		{name: "scope with space", message: "feat(play back): add x", want: errInvalidHeader},
		{name: "scope with slash", message: "feat(web/app): add x", want: errInvalidHeader},
		{name: "bang before scope", message: "feat!(x): add x", want: errInvalidHeader},
		{name: "leading space", message: " feat: add x", want: errInvalidHeader},
		{name: "free-form merge", message: "Merge stuff", want: errInvalidHeader},
		{name: "merge into without a source", message: "Merge into main", want: errInvalidHeader},
		{name: "revert without quotes", message: "Revert feat: add x", want: errInvalidHeader},
		{name: "fixup without space", message: "fixup!feat: add x", want: errInvalidHeader},
		{name: "only trailers look like a header", message: "Signed-off-by: A <a@example.com>", want: errInvalidHeader},
		{name: "header continues on the next line", message: "feat: add x\nand y\n", want: errMissingBlankLine},
		{
			name:    "trailer directly below the header",
			message: "feat: add x\nCo-Authored-By: B <b@example.com>\n",
			want:    errMissingBlankLine,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := checkMessage(test.message)
			if !errors.Is(err, test.want) {
				t.Fatalf("checkMessage(%q) = %v, want %v", test.message, err, test.want)
			}
		})
	}
}

func TestCheckMessageErrorShowsTheExpectedFormat(t *testing.T) {
	t.Parallel()

	err := checkMessage("update stuff")
	if err == nil {
		t.Fatal("checkMessage(\"update stuff\") = nil, want an error")
	}

	for _, want := range []string{
		`"update stuff"`,
		"type(scope)!: subject",
		conventionalTypes,
		"feat(playback): add HLS seeking",
		`Revert "…"`,
		`Reapply "…"`,
		"fixup! …",
		"squash! …",
		"amend! …",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}
