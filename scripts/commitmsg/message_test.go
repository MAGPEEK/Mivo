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
			// Only the header and the line below it are checked, so trailers need no special
			// handling and stay part of the body.
			name:       "trailers stay in the body",
			raw:        "feat: x\n\nbody\n\nSigned-off-by: A <a@example.com>\n",
			wantHeader: "feat: x",
			wantBody:   []string{"", "body", "", "Signed-off-by: A <a@example.com>"},
		},
		{
			name:       "leading and trailing blank lines are removed",
			raw:        "\n\nfeat: x\n\nbody\n\n\n",
			wantHeader: "feat: x",
			wantBody:   []string{"", "body"},
		},
		{
			// git's core.commentChar is not read: lines with another comment character are
			// message text.
			name:       "only the default comment character is stripped",
			raw:        "feat: x\n\n; comment\n",
			wantHeader: "feat: x",
			wantBody:   []string{"", "; comment"},
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
