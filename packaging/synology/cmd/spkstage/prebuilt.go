package main

import (
	"debug/elf"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
)

// File names of the FFmpeg build (ffmpeg/fetch.sh writes them into its destination).
const (
	ffmpegName       = "ffmpeg"
	ffprobeName      = "ffprobe"
	sourceNoticeName = "SOURCE.txt"
)

// minNoticeFiles counts SOURCE.txt plus at least one licence text.
const minNoticeFiles = 2

// executableBits are the permission bits of which at least one marks a file as executable.
const executableBits = 0o111

var (
	// licenseName limits licence file names to characters that are safe in a tar member and
	// an sh word.
	licenseName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	// interimStatus finds the status line fetch.sh writes for an interim FFmpeg build, which is
	// fine for development but must not be released (ffmpeg/README.md).
	interimStatus = regexp.MustCompile(`(?m)^Status:[ \t]+interim[ \t]*\r?$`)
)

var (
	// errWrongMachine reports a prebuilt file that is not an ELF file of the family.
	errWrongMachine = errors.New("wrong ELF machine")
	// errNotExecutable reports a prebuilt executable without an executable bit.
	errNotExecutable = errors.New("not executable")
	// errNotExecutableType reports an ELF file that is neither ET_EXEC nor ET_DYN.
	errNotExecutableType = errors.New("not an ELF executable")
	// errDynamicallyLinked reports an executable with a program interpreter (PT_INTERP): it
	// needs the C library of the build system, which DSM does not provide.
	errDynamicallyLinked = errors.New("dynamically linked executable")
)

// elfIdentity is the ELF class and machine of a family's executables.
type elfIdentity struct {
	class   elf.Class
	machine elf.Machine
}

func (identity elfIdentity) String() string {
	return fmt.Sprintf("%s %s", identity.class, identity.machine)
}

// familyELF returns the identity SynoBuildConf/build expects for a family.
func familyELF(family string) (elfIdentity, error) {
	switch family {
	case "x86_64":
		return elfIdentity{class: elf.ELFCLASS64, machine: elf.EM_X86_64}, nil
	case "armv8":
		return elfIdentity{class: elf.ELFCLASS64, machine: elf.EM_AARCH64}, nil
	case "armv7":
		return elfIdentity{class: elf.ELFCLASS32, machine: elf.EM_ARM}, nil
	default:
		return elfIdentity{}, fmt.Errorf("-family %q has no ELF machine: %w", family, errUnexpectedInput)
	}
}

// prebuiltEntries checks and plans the server binary, ffmpeg, ffprobe and the FFmpeg notices.
func prebuiltEntries(opts options, id string, want elfIdentity, stderr io.Writer) ([]entry, error) {
	dynamic, err := checkExecutable("-binary", opts.binary, want)
	if err != nil {
		return nil, err
	}

	if dynamic {
		return nil, fmt.Errorf("-binary %s has a program interpreter; build it with CGO_ENABLED=0: %w",
			opts.binary, errDynamicallyLinked)
	}

	ffmpegEntries, err := ffmpegDirEntries(opts.ffmpegDir, want, stderr)
	if err != nil {
		return nil, err
	}

	server := entry{path: prebuiltBinDir + "/" + id, mode: executableMode, source: opts.binary}

	return append([]entry{server}, ffmpegEntries...), nil
}

// checkExecutable refuses a missing file, a file without an executable bit and an ELF file of
// another class or machine or of a type other than ET_EXEC and ET_DYN. It reports whether the
// file is dynamically linked (has PT_INTERP); a static-pie is ET_DYN without one. Windows file
// systems carry no executable bit, so that check only runs elsewhere (CI and SPK-07's Docker
// stage).
func checkExecutable(label, file string, want elfIdentity) (bool, error) {
	if err := requireRegularFile(label, file); err != nil {
		return false, err
	}

	info, err := os.Stat(file)
	if err != nil {
		return false, fmt.Errorf("%s %s: %w", label, file, err)
	}

	if runtime.GOOS != "windows" && info.Mode().Perm()&executableBits == 0 {
		return false, fmt.Errorf("%s %s: %w", label, file, errNotExecutable)
	}

	facts, err := readELF(file)
	if err != nil {
		return false, fmt.Errorf("%s %s is not an ELF executable for %s: %w", label, file, want, errWrongMachine)
	}

	if facts.identity != want {
		return false, fmt.Errorf("%s %s is %s, the family needs %s: %w", label, file, facts.identity, want, errWrongMachine)
	}

	if facts.fileType != elf.ET_EXEC && facts.fileType != elf.ET_DYN {
		return false, fmt.Errorf("%s %s is of type %s: %w", label, file, facts.fileType, errNotExecutableType)
	}

	return facts.interpreter, nil
}

// elfFacts are the header fields checkExecutable judges.
type elfFacts struct {
	identity    elfIdentity
	fileType    elf.Type
	interpreter bool
}

func readELF(file string) (elfFacts, error) {
	executable, err := elf.Open(file)
	if err != nil {
		return elfFacts{}, fmt.Errorf("open ELF: %w", err)
	}

	facts := elfFacts{
		identity: elfIdentity{class: executable.Class, machine: executable.Machine},
		fileType: executable.Type,
		interpreter: slices.ContainsFunc(executable.Progs, func(program *elf.Prog) bool {
			return program.Type == elf.PT_INTERP
		}),
	}

	if err := executable.Close(); err != nil {
		return elfFacts{}, fmt.Errorf("close ELF: %w", err)
	}

	return facts, nil
}

// ffmpegDirEntries plans ffmpeg, ffprobe and every licence file of the directory fetch.sh
// wrote. Hidden files (its .fetch-manifest) are skipped; anything else that is not a regular
// file is refused, so nothing unexpected reaches the package.
func ffmpegDirEntries(directory string, want elfIdentity, stderr io.Writer) ([]entry, error) {
	if err := requireDirectory("-ffmpeg-dir", directory); err != nil {
		return nil, err
	}

	licenses, err := licenseEntries(directory)
	if err != nil {
		return nil, err
	}

	interim, err := checkNotices(directory, len(licenses), stderr)
	if err != nil {
		return nil, err
	}

	binaries, err := ffmpegBinaries(directory, want, interim, stderr)
	if err != nil {
		return nil, err
	}

	return append(binaries, licenses...), nil
}

func licenseEntries(directory string) ([]entry, error) {
	items, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("-ffmpeg-dir %s: %w", directory, err)
	}

	var licenses []entry

	for _, item := range items {
		name := item.Name()
		if strings.HasPrefix(name, ".") || name == ffmpegName || name == ffprobeName {
			continue
		}

		if !item.Type().IsRegular() || !licenseName.MatchString(name) {
			return nil, fmt.Errorf("-ffmpeg-dir %s: %s is not a licence file: %w", directory, name, errUnexpectedInput)
		}

		licenses = append(licenses, entry{
			path: ffmpegLicenseDir + "/" + name, mode: regularMode, source: filepath.Join(directory, name),
		})
	}

	return licenses, nil
}

// ffmpegBinaries checks ffmpeg and ffprobe. A dynamically linked build is refused unless it is
// interim: the interim pins of ffmpeg/sources.lock link glibc dynamically and stay usable for
// development until SPK-16 switches to the own static build.
func ffmpegBinaries(directory string, want elfIdentity, interim bool, stderr io.Writer) ([]entry, error) {
	names := []string{ffmpegName, ffprobeName}
	binaries := make([]entry, 0, len(names))

	for _, name := range names {
		source := filepath.Join(directory, name)

		dynamic, err := checkExecutable("-ffmpeg-dir", source, want)
		if err != nil {
			return nil, err
		}

		if dynamic && !interim {
			return nil, fmt.Errorf("-ffmpeg-dir %s has a program interpreter and the build is not interim: %w",
				source, errDynamicallyLinked)
		}

		if dynamic {
			if err := warn(stderr, "%s is dynamically linked; accepted only because the FFmpeg build is interim", name); err != nil {
				return nil, err
			}
		}

		binaries = append(binaries, entry{path: prebuiltBinDir + "/" + name, mode: executableMode, source: source})
	}

	return binaries, nil
}

// checkNotices requires SOURCE.txt and at least one licence text next to it (GPL compliance,
// ADR 0026 section 4) and reports, with a warning, whether the build is interim.
func checkNotices(directory string, files int, stderr io.Writer) (bool, error) {
	noticePath := filepath.Join(directory, sourceNoticeName)
	if err := requireRegularFile("-ffmpeg-dir", noticePath); err != nil {
		return false, err
	}

	if files < minNoticeFiles {
		return false, fmt.Errorf("-ffmpeg-dir %s holds no licence file besides %s: %w", directory, sourceNoticeName, errUnexpectedInput)
	}

	notice, err := os.ReadFile(noticePath) //nolint:gosec // the FFmpeg directory is an input of the build
	if err != nil {
		return false, fmt.Errorf("-ffmpeg-dir %s: %w", noticePath, err)
	}

	if !interimStatus.Match(notice) {
		return false, nil
	}

	return true, warn(stderr, "%s marks an interim FFmpeg build; do not release this package", noticePath)
}

func warn(stderr io.Writer, format string, arguments ...any) error {
	if _, err := fmt.Fprintf(stderr, "spkstage: warning: "+format+"\n", arguments...); err != nil {
		return fmt.Errorf("write warning: %w", err)
	}

	return nil
}

// requireRegularFile names the flag when a required file is missing or not a regular file.
func requireRegularFile(label, file string) error {
	info, err := os.Stat(file)
	if err != nil {
		return fmt.Errorf("%s %s: %w", label, file, err)
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s %s is not a regular file: %w", label, file, errUnexpectedInput)
	}

	return nil
}

// requireDirectory names the flag when a required directory is missing.
func requireDirectory(label, directory string) error {
	info, err := os.Stat(directory)
	if err != nil {
		return fmt.Errorf("%s %s: %w", label, directory, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("%s %s is not a directory: %w", label, directory, errUnexpectedInput)
	}

	return nil
}
