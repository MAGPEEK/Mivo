// Package buildinfo is the single source of version and build metadata of the binary.
package buildinfo

import (
	"runtime"
	"runtime/debug"
	"strings"
)

const (
	// devVersion is reported when neither the linker nor the go command stamped a version.
	devVersion = "0.0.0-dev"

	// develModuleVersion is what the go command reports for unversioned main modules.
	develModuleVersion = "(devel)"

	// shortCommitLength matches the hash length Go uses in pseudo-versions.
	shortCommitLength = 12

	settingRevision = "vcs.revision"
	settingTime     = "vcs.time"
	settingModified = "vcs.modified"
	modifiedTrue    = "true"
)

// version is overridden at link time for release builds:
// -ldflags "-X github.com/magpeek/mivo/internal/platform/buildinfo.version=1.2.3".
var version string //nolint:gochecknoglobals // set by the linker

// Info describes the running binary.
type Info struct {
	// Version is the release version, the module pseudo-version or 0.0.0-dev.
	Version string
	// Commit is the full VCS revision, empty when the build was not stamped.
	Commit string
	// CommitTime is the commit time in RFC 3339 as reported by the go command.
	CommitTime string
	// Modified reports whether the working tree had uncommitted changes at build time.
	Modified bool
	// GoVersion is the Go toolchain version the binary was built with.
	GoVersion string
}

// Read returns the build metadata of the running binary.
func Read() Info {
	build, ok := debug.ReadBuildInfo()
	if !ok {
		build = &debug.BuildInfo{GoVersion: runtime.Version()}
	}

	return resolve(build, version)
}

// Summary formats the version with commit, modified flag and Go version, for example
// "1.2.3 (commit abcdef123456, modified, go1.27.0)".
func (info Info) Summary() string {
	var details []string
	if info.Commit != "" {
		details = append(details, "commit "+shortCommit(info.Commit))
	}

	if info.Modified {
		details = append(details, "modified")
	}

	if info.GoVersion != "" {
		details = append(details, info.GoVersion)
	}

	if len(details) == 0 {
		return info.Version
	}

	return info.Version + " (" + strings.Join(details, ", ") + ")"
}

// resolve derives Info from the build info and the linker override. It is pure so the
// resolution order (override > main module version > devVersion) is testable.
func resolve(build *debug.BuildInfo, override string) Info {
	info := Info{Version: resolveVersion(build, override)}
	if build == nil {
		return info
	}

	info.GoVersion = build.GoVersion

	for _, setting := range build.Settings {
		switch setting.Key {
		case settingRevision:
			info.Commit = setting.Value
		case settingTime:
			info.CommitTime = setting.Value
		case settingModified:
			// The go command writes "true" or "false"; anything else counts as unmodified.
			info.Modified = setting.Value == modifiedTrue
		}
	}

	return info
}

func resolveVersion(build *debug.BuildInfo, override string) string {
	if override != "" {
		return override
	}

	if build != nil && build.Main.Version != "" && build.Main.Version != develModuleVersion {
		return build.Main.Version
	}

	return devVersion
}

func shortCommit(commit string) string {
	if len(commit) <= shortCommitLength {
		return commit
	}

	return commit[:shortCommitLength]
}
