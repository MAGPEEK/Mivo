package main

import (
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/magpeek/mivo/packaging/synology/internal/spkicon"
	"github.com/magpeek/mivo/packaging/synology/internal/spkmeta"
)

// Modes of the staged tree (task SPK-14): scripts and executables, everything else, and
// directories. SynoBuildConf/install sets the modes of the SPK again, because a Windows bind
// mount shows every staged file as 0777.
const (
	executableMode fs.FileMode = 0o755
	regularMode    fs.FileMode = 0o644
	directoryMode  fs.FileMode = 0o755
)

// Paths of the staged project, slash-separated and relative to the project directory
// (packaging/synology/toolkit/README.md, "Staged project").
const (
	buildEnvPath        = "build.env"
	packageEnvPath      = "scripts/package.env"
	licensePath         = "LICENSE"
	prebuiltBinDir      = "prebuilt/bin"
	ffmpegLicenseDir    = "prebuilt/share/licenses/ffmpeg"
	portConfDir         = "prebuilt/port_conf"
	serviceFileSuffix   = ".sc"
	packageTemplatesDir = "packaging/synology/package"
)

// fileCheck is a content rule applied to a staged text file before anything is written.
type fileCheck int

const (
	// checkText refuses CR characters and leftover @NAME@ tokens.
	checkText fileCheck = 1 << iota
	// checkASCII refuses non-ASCII bytes: the chroot reads these files with an ASCII locale.
	checkASCII
	// checkJSON refuses content that does not parse as JSON.
	checkJSON
)

// entry is one file of the staged project: generated or rendered content, or a file copied
// unchanged from source.
type entry struct {
	// path is slash-separated and relative to the project directory.
	path string
	mode fs.FileMode
	// content is the file content; nil means "copy source".
	content []byte
	source  string
	checks  fileCheck
}

// staged summarises a finished staging run.
type staged struct {
	files      int
	dsmVersion string
}

// stage validates every input, plans the project and writes it. Nothing is written unless all
// inputs pass.
func stage(opts options, pkg spkmeta.Package, stderr io.Writer) (staged, error) {
	build, err := spkmeta.NewBuild(pkg, spkmeta.BuildOptions{
		ServerVersion:   opts.version,
		PackageRevision: opts.packageRevision,
		Family:          opts.family,
	})
	if err != nil {
		return staged{}, fmt.Errorf("-family %q, -version %q, -package-revision %d: %w",
			opts.family, opts.version, opts.packageRevision, err)
	}

	entries, err := planProject(opts, build, stderr)
	if err != nil {
		return staged{}, err
	}

	if err := checkEntries(entries); err != nil {
		return staged{}, err
	}

	target, err := checkReplaceable(opts.out, stageInputs(opts), opts.force)
	if err != nil {
		return staged{}, err
	}

	if err := writeAtomically(target, entries, opts.modTime); err != nil {
		return staged{}, err
	}

	return staged{files: len(entries), dsmVersion: build.Version.DSM}, nil
}

// planProject collects every file of the project in a fixed order.
func planProject(opts options, build spkmeta.Build, stderr io.Writer) ([]entry, error) {
	want, err := familyELF(build.Family.Name)
	if err != nil {
		return nil, err
	}

	prebuilt, err := prebuiltEntries(opts, build.Package.ID, want, stderr)
	if err != nil {
		return nil, err
	}

	templates, err := templateEntries(filepath.Join(opts.repo, filepath.FromSlash(packageTemplatesDir)), build)
	if err != nil {
		return nil, err
	}

	generated, err := generatedEntries(build)
	if err != nil {
		return nil, err
	}

	licenseSource := filepath.Join(opts.repo, licensePath)
	if err := requireRegularFile("-repo", licenseSource); err != nil {
		return nil, err
	}

	entries := append(templates, generated...)
	entries = append(entries, prebuilt...)
	entries = append(entries, entry{path: licensePath, mode: regularMode, source: licenseSource})

	return entries, rejectDuplicates(entries)
}

// generatedEntries renders build.env, package.env and the icons.
func generatedEntries(build spkmeta.Build) ([]entry, error) {
	buildEnv, err := build.BuildEnv()
	if err != nil {
		return nil, fmt.Errorf("render %s: %w", buildEnvPath, err)
	}

	packageEnv, err := build.Package.PackageEnv()
	if err != nil {
		return nil, fmt.Errorf("render %s: %w", packageEnvPath, err)
	}

	entries := []entry{
		{path: projectMarkerPath, mode: regularMode, content: []byte(projectMarkerText), checks: checkText},
		{path: buildEnvPath, mode: regularMode, content: []byte(buildEnv), checks: checkText},
		{path: packageEnvPath, mode: regularMode, content: []byte(packageEnv), checks: checkText},
	}

	for _, icon := range spkicon.PackageIcons() {
		png, err := spkicon.EncodePNG(icon.Size)
		if err != nil {
			return nil, fmt.Errorf("render %s: %w", icon.FileName, err)
		}

		entries = append(entries, entry{path: icon.FileName, mode: regularMode, content: png})
	}

	return entries, nil
}

// rejectDuplicates refuses two sources for one path, for example a package.env template left
// next to the generated one. Paths are compared without case: on a Windows checkout or bind
// mount two such files would overwrite each other.
func rejectDuplicates(entries []entry) error {
	seen := make(map[string]bool, len(entries))

	for _, item := range entries {
		key := strings.ToLower(item.path)
		if seen[key] {
			return fmt.Errorf("two sources for %s in the staged project: %w", item.path, errUnexpectedInput)
		}

		seen[key] = true
	}

	return nil
}
