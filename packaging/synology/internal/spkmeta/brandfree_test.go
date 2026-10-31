package spkmeta_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/platform/brand"
)

// TestNoProductNameOutsideTests enforces ADR 0002 for this package: the product name reaches
// the package files only through internal/platform/brand. Every string literal and comment of
// the non-test files is checked; import paths are skipped because the module path itself
// carries the working title.
func TestNoProductNameOutsideTests(t *testing.T) {
	t.Parallel()

	names := []string{strings.ToLower(brand.ID), strings.ToLower(brand.DisplayName)}

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("list package files: %v", err)
	}

	checked := 0

	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}

		checked++

		for _, text := range literalsAndComments(t, path) {
			for _, name := range names {
				if strings.Contains(strings.ToLower(text), name) {
					t.Errorf("%s contains the product name in %s", path, text)

					break
				}
			}
		}
	}

	if checked == 0 {
		t.Fatal("no non-test Go file found")
	}
}

func literalsAndComments(t *testing.T, path string) []string {
	t.Helper()

	// The parser reads the file itself; the paths come from globbing the package directory.
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	imports := map[*ast.BasicLit]bool{}
	for _, spec := range file.Imports {
		imports[spec.Path] = true
	}

	var texts []string

	ast.Inspect(file, func(node ast.Node) bool {
		if literal, ok := node.(*ast.BasicLit); ok && literal.Kind == token.STRING && !imports[literal] {
			texts = append(texts, literal.Value)
		}

		return true
	})

	for _, group := range file.Comments {
		texts = append(texts, group.Text())
	}

	return texts
}
