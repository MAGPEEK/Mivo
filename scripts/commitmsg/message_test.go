package main

import (
	"slices"
	"testing"
)

func TestParseMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		raw        string
		wantHeader string
		wantBody   []string
	}{
		{name: "empty", raw: "", wantHeader: "", wantBody: nil},
		{name: "header only", raw: "feat: x\n", wantHeader: "feat: x", wantBody: nil},
		{
			name:       "trailing whitespace is trimmed",
			raw:        "feat: x  \t\n\nbody  \n",
			wantHeader: "feat: x",
			wantBody:   []string{"", "body"},
		},
		{
			name:       "trailer block is removed",
			raw:        "feat: x\n\nbody\n\nSigned-off-by: A <a@example.com>\n",
			wantHeader: "feat: x",
			wantBody:   []string{"", "body"},
		},
		{
			name:       "folded trailer value is part of the block",
			raw:        "feat: x\n\nNote-To-Reviewer: first line\n  continued\nCo-Authored-By: B <b@example.com>\n",
			wantHeader: "feat: x",
			wantBody:   nil,
		},
		{
			name:       "last paragraph with prose is kept",
			raw:        "feat: x\n\nSee: the plan\nThis line is prose.\n",
			wantHeader: "feat: x",
			wantBody:   []string{"", "See: the plan", "This line is prose."},
		},
		{
			name:       "continuation line cannot start a trailer block",
			raw:        "feat: x\n\n  indented: text\n",
			wantHeader: "feat: x",
			wantBody:   []string{"", "  indented: text"},
		},
		{
			name:       "token with invalid characters is no trailer",
			raw:        "feat: x\n\nSee the plan: section 3\n",
			wantHeader: "feat: x",
			wantBody:   []string{"", "See the plan: section 3"},
		},
		{
			name:       "token without value is no trailer",
			raw:        "feat: x\n\nNote:\n",
			wantHeader: "feat: x",
			wantBody:   []string{"", "Note:"},
		},
		{
			name:       "single paragraph is never a trailer block",
			raw:        "chore: hook test\n",
			wantHeader: "chore: hook test",
			wantBody:   nil,
		},
		{
			name:       "comments and scissors section are removed",
			raw:        "# comment\nfeat: x\n" + scissorsLine + "\n+diff\n",
			wantHeader: "feat: x",
			wantBody:   nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := parseMessage(test.raw)
			if got.header != test.wantHeader {
				t.Errorf("header = %q, want %q", got.header, test.wantHeader)
			}

			if !slices.Equal(got.body, test.wantBody) {
				t.Errorf("body = %q, want %q", got.body, test.wantBody)
			}
		})
	}
}
