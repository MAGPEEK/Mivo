package archtest

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	goFileSuffix   = ".go"
	testFileSuffix = "_test.go"
	goModFile      = "go.mod"
	internalDir    = "internal"
	testdataDir    = "testdata"
)

// treeReport is the result of checking every production file below internal/.
type treeReport struct {
	checkedFiles int
	violations   []Violation
}

// checkTree checks every non-test Go file below moduleRoot/internal against layers.
func checkTree(moduleRoot string, layers []layer) (treeReport, error) {
	var report treeReport

	walkErr := filepath.WalkDir(filepath.Join(moduleRoot, internalDir),
		func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return fmt.Errorf("walk %s: %w", path, err)
			}

			if entry.IsDir() {
				return skipIgnoredDirectory(entry.Name())
			}

			if !isProductionGoFile(entry.Name()) {
				return nil
			}

			violations, err := checkFile(moduleRoot, path, layers)
			if err != nil {
				return err
			}

			report.checkedFiles++
			report.violations = append(report.violations, violations...)

			return nil
		})
	if walkErr != nil {
		return treeReport{}, fmt.Errorf("check tree %s: %w", moduleRoot, walkErr)
	}

	return report, nil
}

// skipIgnoredDirectory skips the directories the go command ignores as well.
func skipIgnoredDirectory(name string) error {
	if name == testdataDir || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
		return filepath.SkipDir
	}

	return nil
}

func isProductionGoFile(name string) bool {
	return strings.HasSuffix(name, goFileSuffix) && !strings.HasSuffix(name, testFileSuffix)
}

func checkFile(moduleRoot, path string, layers []layer) ([]Violation, error) {
	relPath, err := filepath.Rel(moduleRoot, path)
	if err != nil {
		return nil, fmt.Errorf("relative path of %s: %w", path, err)
	}

	source, err := os.ReadFile(path) //nolint:gosec // path comes from walking the module's own source tree
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	return checkSource(filepath.ToSlash(relPath), source, layers)
}

// errModuleRootNotFound is returned when no go.mod exists above the start directory.
var errModuleRootNotFound = errors.New("module root not found")

// findModuleRoot walks up from start to the directory containing go.mod.
func findModuleRoot(start string) (string, error) {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, goModFile)); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("search %s from %s: %w", goModFile, start, errModuleRootNotFound)
		}

		dir = parent
	}
}
