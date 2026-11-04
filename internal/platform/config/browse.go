package config

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/magpeek/mivo/internal/platform/paths"
)

// Keys of the folder browser and installation settings (ADR 0028).
const (
	keyBrowseRoots = "browse.roots"
	keyInstallKind = "install.kind"
)

// Installation kinds. They only select defaults of the surrounding packaging (M3-19l,
// SPK-17); the server behaves the same otherwise.
const (
	InstallKindStandard = "standard"
	InstallKindDocker   = "docker"
	InstallKindSynology = "synology"
)

const (
	goosWindows = "windows"
	// windowsDriveLetters are the drives probed for default browse roots.
	windowsDriveLetters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	envHomeVariable     = "HOME"
)

var errDuplicateBrowseRoot = errors.New("duplicate browse root")

// Browse holds the folder browser settings (ADR 0028).
type Browse struct {
	// Roots are the absolute, unique directories the folder browser may start at. They need
	// not exist; the browser reports missing ones as unavailable. Never nil after Load.
	Roots []string
}

// Install describes the kind of installation.
type Install struct {
	// Kind is InstallKindStandard, InstallKindDocker or InstallKindSynology.
	Kind string
}

func applyInstallKind(cfg *Config, raw, _ string) error {
	kind, err := parseChoice(raw, InstallKindStandard, InstallKindDocker, InstallKindSynology)
	if err != nil {
		return err
	}

	cfg.Install.Kind = kind

	return nil
}

// applyBrowseRoots resolves the raw roots against baseDir and stores them as a whole.
func applyBrowseRoots(cfg *Config, raws []string, baseDir string) error {
	roots := make([]string, 0, len(raws))

	for index, raw := range raws {
		root, err := resolvePath(raw, baseDir)
		if err != nil {
			return fmt.Errorf("%s: %w", listItemLabel(keyBrowseRoots, index), err)
		}

		roots = append(roots, root)
	}

	cfg.Browse.Roots = roots

	return nil
}

// environmentBrowseRoots applies <PREFIX>_BROWSE_ROOTS; an empty value means no roots.
func environmentBrowseRoots(cfg *Config, env paths.Environment, baseDir string) error {
	name := envName(keyBrowseRoots)

	raw, ok := env.Lookup(name)
	if !ok {
		return nil
	}

	if err := applyBrowseRoots(cfg, filepath.SplitList(raw), baseDir); err != nil {
		return fmt.Errorf("environment variable %s: %w", name, err)
	}

	return nil
}

// finalizeBrowseRoots fills in the defaults when no source set the roots, then validates.
func finalizeBrowseRoots(cfg *Config, options Options) error {
	if cfg.Browse.Roots == nil {
		exists := options.PathExists
		if exists == nil {
			exists = directoryExists
		}

		cfg.Browse.Roots = DefaultBrowseRoots(options.Environment, cfg.Install.Kind, exists)
	}

	return checkBrowseRoots(cfg.Browse.Roots, options.Environment.GOOS)
}

// checkBrowseRoots rejects duplicates after cleaning; case-insensitive on Windows. Two paths
// that each lie within the other are equal, which is why Within is asked both ways.
func checkBrowseRoots(roots []string, goos string) error {
	for index, root := range roots {
		for _, earlier := range roots[:index] {
			if paths.Within(goos, root, earlier) && paths.Within(goos, earlier, root) {
				return fmt.Errorf("%s: %w: %w: %q", keyBrowseRoots, ErrInvalid, errDuplicateBrowseRoot, root)
			}
		}
	}

	return nil
}

// DefaultBrowseRoots returns the browse roots used when the key is absent: every present
// drive root on Windows; /media, /mnt, /srv and the home directory that exist elsewhere.
// The home directory is only a default for standard installs: on a NAS or in a container it
// may hold secrets or the data directory, and in containers it is often "/". It is also
// skipped when relative, the filesystem root or already listed. exists is injected so the
// result is testable on any OS.
func DefaultBrowseRoots(env paths.Environment, installKind string, exists func(path string) bool) []string {
	roots := []string{}

	if env.GOOS == goosWindows {
		for _, letter := range windowsDriveLetters {
			if drive := string(letter) + `:\`; exists(drive) {
				roots = append(roots, drive)
			}
		}

		return roots
	}

	candidates := []string{"/media", "/mnt", "/srv"}
	if installKind == InstallKindStandard {
		if home, ok := homeCandidate(env); ok && !slices.Contains(candidates, home) {
			candidates = append(candidates, home)
		}
	}

	for _, candidate := range candidates {
		if exists(candidate) {
			roots = append(roots, candidate)
		}
	}

	return roots
}

// homeCandidate returns the cleaned absolute home directory, never the filesystem root.
// Slash paths: this branch only serves non-Windows hosts.
func homeCandidate(env paths.Environment) (string, bool) {
	value, ok := env.Lookup(envHomeVariable)
	if !ok || strings.TrimSpace(value) == "" {
		return "", false
	}

	home := path.Clean(value)

	return home, path.IsAbs(home) && path.Dir(home) != home
}

// directoryExists is the default existence probe.
func directoryExists(path string) bool {
	info, err := os.Stat(path)

	return err == nil && info.IsDir()
}
