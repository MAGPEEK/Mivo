package nfo_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestParserHasNoNetworkOrFileAccess guards ADR 0027 §7: the parser takes bytes only, so
// no URL in an NFO can ever be requested and no path in it can be opened.
func TestParserHasNoNetworkOrFileAccess(t *testing.T) {
	t.Parallel()

	forbidden := []string{"net", "net/http", "os", "os/exec", "io/fs", "path/filepath", "syscall"}

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}

	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}

		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}

		for _, spec := range parsed.Imports {
			path, _ := strconv.Unquote(spec.Path.Value)
			for _, denied := range forbidden {
				if path == denied {
					t.Errorf("%s imports %s", file, path)
				}
			}
		}
	}
}
