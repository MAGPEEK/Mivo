package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/magpeek/mivo/packaging/synology/internal/spkmeta"
)

// errUnexpectedInput reports a file the staging step has no rule for, or an input in the
// wrong form.
var errUnexpectedInput = errors.New("unexpected input")

// Top-level template paths under packaging/synology/package/.
const (
	infoScript         = "INFO.sh"
	buildConfDir       = "SynoBuildConf"
	scriptsDir         = "scripts"
	confDir            = "conf"
	wizardDir          = "WIZARD_UIFILES"
	serviceTemplate    = "port/service.sc"
	dependsFile        = "SynoBuildConf/depends"
	wizardScriptSuffix = ".sh"
)

// templateTarget is where a template lands in the project, with its mode and checks.
type templateTarget struct {
	path   string
	mode   fs.FileMode
	checks fileCheck
}

// targetOf maps a template path (slash-separated, relative to packaging/synology/package/) to
// its place in the project. Templates without a rule are refused, so a new file cannot slip
// into the SPK, or out of it, unnoticed.
func targetOf(name, id string) (templateTarget, bool) {
	directory, base := path.Split(name)

	switch {
	case name == infoScript:
		return templateTarget{path: name, mode: executableMode, checks: checkText | checkASCII}, true
	case name == dependsFile:
		return templateTarget{path: name, mode: regularMode, checks: checkText | checkASCII}, true
	case directory == buildConfDir+"/":
		return templateTarget{path: name, mode: executableMode, checks: checkText | checkASCII}, true
	case directory == scriptsDir+"/":
		return templateTarget{path: name, mode: executableMode, checks: checkText}, true
	case directory == confDir+"/":
		return templateTarget{path: name, mode: regularMode, checks: checkText | checkJSON}, true
	case directory == wizardDir+"/" && strings.HasSuffix(base, wizardScriptSuffix):
		return templateTarget{path: name, mode: executableMode, checks: checkText}, true
	case directory == wizardDir+"/":
		return templateTarget{path: name, mode: regularMode, checks: checkText | checkJSON}, true
	case name == serviceTemplate:
		return templateTarget{path: portConfDir + "/" + id + serviceFileSuffix, mode: regularMode, checks: checkText}, true
	default:
		return templateTarget{}, false
	}
}

// templateEntries renders every file under the template directory with spkmeta.
func templateEntries(root string, build spkmeta.Build) ([]entry, error) {
	if err := requireDirectory("-repo", root); err != nil {
		return nil, err
	}

	var entries []entry

	err := filepath.WalkDir(root, func(current string, item fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if item.IsDir() {
			return nil
		}

		rendered, err := renderTemplate(root, current, item, build)
		if err != nil {
			return err
		}

		entries = append(entries, rendered)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("stage templates from %s: %w", root, err)
	}

	return entries, nil
}

func renderTemplate(root, current string, item fs.DirEntry, build spkmeta.Build) (entry, error) {
	relative, err := filepath.Rel(root, current)
	if err != nil {
		return entry{}, fmt.Errorf("locate %s: %w", current, err)
	}

	name := filepath.ToSlash(relative)

	target, known := targetOf(name, build.Package.ID)
	if !known || !item.Type().IsRegular() {
		return entry{}, fmt.Errorf("template %s has no place in the staged project: %w", name, errUnexpectedInput)
	}

	// The path comes from walking the repository's own template directory.
	content, err := os.ReadFile(current) //nolint:gosec // template of the repository, found by WalkDir
	if err != nil {
		return entry{}, fmt.Errorf("read template %s: %w", name, err)
	}

	rendered, err := build.RenderTemplate(name, string(content))
	if err != nil {
		return entry{}, fmt.Errorf("template %s: %w", name, err)
	}

	return entry{path: target.path, mode: target.mode, content: []byte(rendered), checks: target.checks}, nil
}
