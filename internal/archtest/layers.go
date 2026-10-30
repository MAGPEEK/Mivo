// Package archtest enforces the layer matrix of ADR 0019 with go/parser. It duplicates the
// depguard rules on purpose: a plain `go test ./...` reports layering violations even on
// machines without golangci-lint.
package archtest

import (
	"fmt"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
)

// modulePath is the Go module path (ADR 0016); every internal import starts with it.
const modulePath = "github.com/magpeek/mivo"

// importRule matches an import path exactly or, unless exact is set, as a path prefix
// ("net/http" also matches "net/http/httptest" but not "net/httpx").
type importRule struct {
	path  string
	exact bool
}

func (rule importRule) matches(importPath string) bool {
	if importPath == rule.path {
		return true
	}

	return !rule.exact && strings.HasPrefix(importPath, rule.path+"/")
}

// layer lists the import restrictions for every file below dir.
type layer struct {
	// dir is the slash-separated directory relative to the module root, e.g. "internal/app".
	dir string
	// standardLibraryOnly restricts imports to the standard library plus allowedModule.
	standardLibraryOnly bool
	// allowedModule lists the module packages a standardLibraryOnly layer may import.
	allowedModule []importRule
	// denied is checked first and always wins.
	denied []importRule
}

// Violation is a single forbidden import found in a file.
type Violation struct {
	// File is the slash-separated path relative to the module root.
	File string
	// Import is the offending import path.
	Import string
	// Reason names the layer and the broken rule.
	Reason string
}

// String formats the violation for test output.
func (violation Violation) String() string {
	return fmt.Sprintf("%s imports %q: %s", violation.File, violation.Import, violation.Reason)
}

func prefix(path string) importRule  { return importRule{path: path} }
func exactly(path string) importRule { return importRule{path: path, exact: true} }
func module(path string) importRule  { return importRule{path: modulePath + "/" + path} }

// layerMatrix returns the matrix of ADR 0019. Keep it in sync with depguard in .golangci.yml.
func layerMatrix() []layer {
	return []layer{
		{
			dir:                 "internal/domain",
			standardLibraryOnly: true,
			allowedModule:       []importRule{module("internal/domain")},
			denied: []importRule{
				exactly("os"), prefix("os/exec"), exactly("net"), prefix("net/http"),
				prefix("database/sql"), prefix("syscall"),
			},
		},
		{
			dir:                 "internal/app",
			standardLibraryOnly: true,
			allowedModule:       []importRule{module("internal/domain"), module("internal/app")},
			denied:              []importRule{prefix("os/exec"), prefix("net/http"), prefix("database/sql")},
		},
		{
			dir: "internal/platform",
			denied: []importRule{
				module("internal/domain"), module("internal/app"), module("internal/adapters"), module("cmd"),
			},
		},
		{
			dir:    "internal/adapters",
			denied: []importRule{module("cmd")},
		},
	}
}

// checkSource parses the imports of one file and reports the ones its layer forbids.
func checkSource(relPath string, source []byte, layers []layer) ([]Violation, error) {
	file, err := parser.ParseFile(token.NewFileSet(), relPath, source, parser.ImportsOnly)
	if err != nil {
		return nil, fmt.Errorf("parse imports of %s: %w", relPath, err)
	}

	imports := make([]string, 0, len(file.Imports))
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, fmt.Errorf("unquote import %s in %s: %w", spec.Path.Value, relPath, err)
		}

		imports = append(imports, importPath)
	}

	return checkImports(relPath, imports, layers), nil
}

// checkImports reports every import of the file at relPath that its layer forbids. Files
// outside all layers (cmd, archtest) are unrestricted.
func checkImports(relPath string, imports []string, layers []layer) []Violation {
	owner, found := layerOf(relPath, layers)
	if !found {
		return nil
	}

	var violations []Violation

	for _, importPath := range imports {
		if reason, allowed := owner.permits(importPath); !allowed {
			violations = append(violations, Violation{File: relPath, Import: importPath, Reason: reason})
		}
	}

	return violations
}

func layerOf(relPath string, layers []layer) (layer, bool) {
	for _, candidate := range layers {
		if strings.HasPrefix(relPath, candidate.dir+"/") {
			return candidate, true
		}
	}

	return layer{}, false
}

func (owner layer) permits(importPath string) (reason string, allowed bool) {
	if matchesAny(owner.denied, importPath) {
		return owner.dir + " must not import it", false
	}

	if !owner.standardLibraryOnly || isStandardLibrary(importPath) || matchesAny(owner.allowedModule, importPath) {
		return "", true
	}

	return owner.dir + " may only import the standard library and its allowed layers", false
}

func matchesAny(rules []importRule, importPath string) bool {
	for _, rule := range rules {
		if rule.matches(importPath) {
			return true
		}
	}

	return false
}

// isStandardLibrary uses the rule of the go command: standard library paths have no dot in
// their first element, every module path has one.
func isStandardLibrary(importPath string) bool {
	firstElement, _, _ := strings.Cut(importPath, "/")

	return !strings.Contains(firstElement, ".")
}
