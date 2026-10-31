package spkmeta

import (
	"fmt"
	"strconv"
)

// Build is everything one SPK needs: the package values, the version and the family.
type Build struct {
	// Package holds the brand values, texts, port and DSM minimum.
	Package Package
	// Version is the server version and its DSM mapping.
	Version Version
	// Family is the architecture family of the SPK.
	Family Family
}

// BuildOptions selects the version and family of a Build.
type BuildOptions struct {
	// ServerVersion is the server's semantic version, for example "0.1.0-alpha.2".
	ServerVersion string
	// PackageRevision counts packaging-only fixes of the same server version (0–9).
	PackageRevision int
	// Family is the family name: "x86_64", "armv8" or "armv7".
	Family string
}

// NewBuild maps the version, looks up the family and validates the result. The staging step
// passes DefaultPackage; tests pass a different brand.
func NewBuild(pkg Package, options BuildOptions) (Build, error) {
	version, err := MapVersion(options.ServerVersion, options.PackageRevision)
	if err != nil {
		return Build{}, fmt.Errorf("new build: %w", err)
	}

	family, err := LookupFamily(options.Family)
	if err != nil {
		return Build{}, fmt.Errorf("new build: %w", err)
	}

	build := Build{Package: pkg, Version: version, Family: family}
	if err := build.Validate(); err != nil {
		return Build{}, fmt.Errorf("new build: %w", err)
	}

	return build, nil
}

// Validate reports the first value that cannot be written to build.env, INFO or a template.
// It also guards a Build assembled without NewBuild.
func (build Build) Validate() error {
	if err := build.Package.Validate(); err != nil {
		return err
	}

	// pkg_dump_info echoes arch and version unquoted like the main texts, so the stricter
	// rule applies to them as well.
	infoValues := []envAssignment{
		{name: "DSM_VERSION", value: build.Version.DSM},
		{name: "ARCH", value: build.Family.Arch},
	}
	for _, value := range infoValues {
		if err := ValidateINFOMainValue(value.name, value.value); err != nil {
			return err
		}
	}

	if build.Version.Server == "" {
		return fmt.Errorf("VERSION is empty: %w", ErrInvalidValue)
	}

	return validateEnvValue("VERSION", build.Version.Server)
}

// BuildEnv renders build.env, which INFO.sh and SynoBuildConf/* source inside the toolkit
// chroot, with exactly the fields packaging/synology/toolkit/README.md lists.
func (build Build) BuildEnv() (string, error) {
	if err := build.Validate(); err != nil {
		return "", fmt.Errorf("render build.env: %w", err)
	}

	pkg := build.Package

	return renderEnv([]envAssignment{
		{name: "ID", value: pkg.ID},
		{name: "DISPLAY_NAME", value: pkg.DisplayName},
		{name: "DESCRIPTION", value: pkg.Description},
		{name: "DESCRIPTION_GER", value: pkg.DescriptionGerman},
		{name: "MAINTAINER", value: pkg.Maintainer},
		{name: "MAINTAINER_URL", value: pkg.MaintainerURL},
		{name: "VERSION", value: build.Version.Server},
		{name: "DSM_VERSION", value: build.Version.DSM},
		{name: "BETA", value: build.Version.BetaFlag()},
		{name: "OS_MIN_VER", value: pkg.OSMinVersion},
		{name: "ARCH", value: build.Family.Arch},
		{name: "PORT", value: strconv.Itoa(pkg.Port)},
	})
}

// templateValues are the values of the build-time tokens @NAME@ (ADR 0026 section 6).
func (build Build) templateValues() map[string]string {
	return map[string]string{
		"ID":           build.Package.ID,
		"DISPLAY_NAME": build.Package.DisplayName,
		"ENV_PREFIX":   build.Package.EnvPrefix,
		"PORT":         strconv.Itoa(build.Package.Port),
		"VERSION":      build.Version.Server,
		"DSM_VERSION":  build.Version.DSM,
	}
}
