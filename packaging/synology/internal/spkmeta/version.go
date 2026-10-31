package spkmeta

import (
	"fmt"
	"strconv"
	"strings"
)

// Version is the server version together with the version Package Center sees.
type Version struct {
	// Server is the semantic version of the server, for example "0.1.0-alpha.2". The binary
	// and the web app show it; build.env carries it as VERSION.
	Server string
	// DSM is INFO's "version" in the form "<major>.<minor>.<patch>-<build>".
	DSM string
	// Beta is true for every pre-release; INFO then carries beta="yes".
	Beta bool
}

// Beta flag values of INFO and build.env.
const (
	betaYes = "yes"
	betaNo  = "no"
)

// BetaFlag spells the beta flag as INFO and build.env expect it: "yes" or "no".
func (version Version) BetaFlag() string {
	if version.Beta {
		return betaYes
	}

	return betaNo
}

// Build number layout of OD-SPK-5: stage × 1000 + pre-release number × 10 + revision.
const (
	stageWeight         = 1000
	prereleaseWeight    = 10
	maxPrereleaseNumber = 99
	maxPackageRevision  = 9
	// buildNumberDigits pads the build number, so local builds read "0000" like the table of
	// ADR 0026 section 5.
	buildNumberDigits = 4
)

// releaseStage orders the pre-release identifiers below the final release.
type releaseStage int

const (
	stageDevelopment releaseStage = 0
	stageAlpha       releaseStage = 1
	stageBeta        releaseStage = 2
	stageCandidate   releaseStage = 3
	stageFinal       releaseStage = 9
)

// developmentVersion is what internal/platform/buildinfo reports for an unstamped binary.
// Its "dev" identifier sorts between "beta" and "rc" in SemVer, so it is accepted only with
// the 0.0.0 core, where it cannot break the order of real releases.
const (
	developmentVersion    = "0.0.0-dev"
	developmentIdentifier = "dev"
)

// semanticVersion is a parsed server version.
type semanticVersion struct {
	major, minor, patch int64
	stage               releaseStage
	prerelease          int64
}

// MapVersion maps a server version and a package revision (0–9; it counts packaging-only
// fixes of the same server version) to the DSM package version of OD-SPK-5, for example
// ("0.1.0-alpha.2", 1) → "0.1.0-1021". Releases map to build 9000 + revision.
//
// The DSM ordering this relies on (feature numbers from the left, then the build number) is
// unconfirmed (ADR 0026 Notes). If SPK-12's upgrade test fails, dsmBuildNumber is the one
// place to switch to a globally monotonic build number (DSM-NOTES "Impact" 2).
func MapVersion(server string, revision int) (Version, error) {
	parsed, err := parseServerVersion(server)
	if err != nil {
		return Version{}, fmt.Errorf("map server version %q: %w", server, err)
	}

	if revision < 0 || revision > maxPackageRevision {
		return Version{}, fmt.Errorf("map server version %q: package revision %d outside 0..%d: %w",
			server, revision, maxPackageRevision, ErrInvalidVersion)
	}

	dsm := fmt.Sprintf("%d.%d.%d-%0*d", parsed.major, parsed.minor, parsed.patch,
		buildNumberDigits, dsmBuildNumber(parsed, revision))

	return Version{Server: server, DSM: dsm, Beta: parsed.stage != stageFinal}, nil
}

// dsmBuildNumber is the build part of the DSM version (OD-SPK-5 A).
func dsmBuildNumber(version semanticVersion, revision int) int64 {
	return int64(version.stage)*stageWeight + version.prerelease*prereleaseWeight + int64(revision)
}

// parseServerVersion accepts "<major>.<minor>.<patch>" optionally followed by
// "-alpha.N", "-beta.N" or "-rc.N", and the development version "0.0.0-dev".
func parseServerVersion(text string) (semanticVersion, error) {
	if strings.HasPrefix(text, "v") {
		return semanticVersion{}, fmt.Errorf("leading v is not part of the version: %w", ErrInvalidVersion)
	}

	if strings.Contains(text, "+") {
		return semanticVersion{}, fmt.Errorf("build metadata (+...) cannot be mapped: %w", ErrInvalidVersion)
	}

	core, prerelease, hasPrerelease := strings.Cut(text, "-")

	version, err := parseCore(core)
	if err != nil {
		return semanticVersion{}, err
	}

	if !hasPrerelease {
		version.stage = stageFinal

		return version, nil
	}

	if prerelease == developmentIdentifier {
		if text != developmentVersion {
			return semanticVersion{}, fmt.Errorf("%q is only valid as %s: %w",
				developmentIdentifier, developmentVersion, ErrInvalidVersion)
		}

		version.stage = stageDevelopment

		return version, nil
	}

	version.stage, version.prerelease, err = parsePrerelease(prerelease)

	return version, err
}

// coreParts is the number of dot-separated numbers of a version core.
const coreParts = 3

func parseCore(core string) (semanticVersion, error) {
	parts := strings.Split(core, ".")
	if len(parts) != coreParts {
		return semanticVersion{}, fmt.Errorf("core %q is not <major>.<minor>.<patch>: %w", core, ErrInvalidVersion)
	}

	numbers := make([]int64, 0, coreParts)

	for _, part := range parts {
		number, err := parseNumber(part)
		if err != nil {
			return semanticVersion{}, fmt.Errorf("core %q: %w", core, err)
		}

		numbers = append(numbers, number)
	}

	return semanticVersion{major: numbers[0], minor: numbers[1], patch: numbers[2]}, nil
}

func parsePrerelease(prerelease string) (releaseStage, int64, error) {
	identifier, numberText, _ := strings.Cut(prerelease, ".")

	stage, known := prereleaseStage(identifier)
	if !known {
		return 0, 0, fmt.Errorf("unknown pre-release identifier %q (want alpha, beta or rc): %w",
			identifier, ErrInvalidVersion)
	}

	number, err := parseNumber(numberText)
	if err != nil {
		return 0, 0, fmt.Errorf("pre-release number of %q: %w", prerelease, err)
	}

	if number > maxPrereleaseNumber {
		return 0, 0, fmt.Errorf("pre-release number %d of %q above %d: %w",
			number, prerelease, maxPrereleaseNumber, ErrInvalidVersion)
	}

	return stage, number, nil
}

func prereleaseStage(identifier string) (releaseStage, bool) {
	switch identifier {
	case "alpha":
		return stageAlpha, true
	case "beta":
		return stageBeta, true
	case "rc":
		return stageCandidate, true
	default:
		return 0, false
	}
}

// maxVersionNumberBits limits every number to 0..2^31−1, the range the guide allows for each
// part of INFO's version.
const maxVersionNumberBits = 32

// parseNumber accepts a SemVer numeric identifier: ASCII digits without a leading zero.
func parseNumber(text string) (int64, error) {
	if text == "" || strings.Trim(text, "0123456789") != "" {
		return 0, fmt.Errorf("%q is not a number: %w", text, ErrInvalidVersion)
	}

	if len(text) > 1 && text[0] == '0' {
		return 0, fmt.Errorf("%q has a leading zero: %w", text, ErrInvalidVersion)
	}

	number, err := strconv.ParseInt(text, 10, maxVersionNumberBits)
	if err != nil {
		return 0, fmt.Errorf("%q is above 2147483647: %w", text, ErrInvalidVersion)
	}

	return number, nil
}
