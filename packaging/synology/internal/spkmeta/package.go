package spkmeta

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/magpeek/mivo/internal/platform/brand"
	"github.com/magpeek/mivo/internal/platform/config"
)

// Package holds the values of the SPK that do not depend on version or family.
type Package struct {
	// ID is INFO's "package", the binary name, the package user and the .sc section name.
	ID string
	// DisplayName is INFO's "displayname".
	DisplayName string
	// EnvPrefix is the prefix of the server's environment variables (<EnvPrefix>_CONFIG_DIR).
	EnvPrefix string
	// Description is INFO's English "description".
	Description string
	// DescriptionGerman is INFO's "description_ger".
	DescriptionGerman string
	// Maintainer is INFO's "maintainer".
	Maintainer string
	// MaintainerURL is INFO's "maintainer_url".
	MaintainerURL string
	// Port is the server's HTTP port: INFO "adminport" and the firewall service.
	Port int
	// OSMinVersion is INFO's "os_min_ver".
	OSMinVersion string
}

// Package Center texts of the media server running on the NAS (ADR 0026 "What the package
// is"). They carry no product name and obey the INFO rules of ValidateINFOMainValue.
const (
	serverDescription = "Media server for your NAS: scans your video folders and streams them " +
		"to web browsers on the other devices of your home network, converting files with the " +
		"bundled FFmpeg when a device cannot play them directly."
	serverDescriptionGerman = "Medienserver für Ihr NAS: durchsucht Ihre Videoordner und streamt " +
		"sie an Webbrowser auf den anderen Geräten in Ihrem Heimnetz. Kann ein Gerät eine Datei " +
		"nicht direkt abspielen, wandelt das mitgelieferte FFmpeg sie um."
)

const (
	// maintainer is INFO's "maintainer" (ADR 0026 section 5).
	maintainer = "MAGPEEK"
	// minimumDSMVersion is DSM 7.2-64570, the first 7.2 build every 7.2 model received
	// (ADR 0026 Notes "INFO").
	minimumDSMVersion = "7.2-64570"
)

// DefaultPackage returns the package values of the real product: brand values from
// internal/platform/brand and the server's default port.
func DefaultPackage() Package {
	return Package{
		ID:                brand.ID,
		DisplayName:       brand.DisplayName,
		EnvPrefix:         brand.EnvPrefix,
		Description:       serverDescription,
		DescriptionGerman: serverDescriptionGerman,
		Maintainer:        maintainer,
		MaintainerURL:     brand.RepositoryURL,
		Port:              config.DefaultPort,
		OSMinVersion:      minimumDSMVersion,
	}
}

// Valid TCP port range.
const (
	minPort = 1
	maxPort = 65535
)

// Formats of the identifiers. The ID becomes a DSM package name, a user name and the section
// name of the .sc file, which allows letters, digits, "-" and "." but no underscore
// (DSM-NOTES); lower case keeps it equal to the binary name. The prefix starts environment
// variable names.
const (
	packageIDPattern    = `^[a-z0-9][a-z0-9.-]*$`
	envPrefixPattern    = `^[A-Z][A-Z0-9_]*$`
	osMinVersionPattern = `^[0-9]+\.[0-9]+-[0-9]+$`
)

// Validate reports the first value that cannot be written to INFO, build.env, package.env or
// a template.
func (pkg Package) Validate() error {
	formats := []struct {
		name, value, pattern string
	}{
		{name: "ID", value: pkg.ID, pattern: packageIDPattern},
		{name: "ENV_PREFIX", value: pkg.EnvPrefix, pattern: envPrefixPattern},
		{name: "OS_MIN_VER", value: pkg.OSMinVersion, pattern: osMinVersionPattern},
	}

	for _, format := range formats {
		if !regexp.MustCompile(format.pattern).MatchString(format.value) {
			return fmt.Errorf("%s %q does not match %s: %w", format.name, format.value, format.pattern, ErrInvalidValue)
		}
	}

	if pkg.Port < minPort || pkg.Port > maxPort {
		return fmt.Errorf("PORT %d outside %d..%d: %w", pkg.Port, minPort, maxPort, ErrInvalidValue)
	}

	for _, text := range pkg.texts() {
		if err := ValidateINFOMainValue(text.name, text.value); err != nil {
			return err
		}

		if err := validateEnvValue(text.name, text.value); err != nil {
			return err
		}
	}

	return nil
}

// texts are the free-text fields; INFO prints them through pkg_dump_info's main field loop.
func (pkg Package) texts() []envAssignment {
	return []envAssignment{
		{name: "DISPLAY_NAME", value: pkg.DisplayName},
		{name: "DESCRIPTION", value: pkg.Description},
		{name: "DESCRIPTION_GER", value: pkg.DescriptionGerman},
		{name: "MAINTAINER", value: pkg.Maintainer},
		{name: "MAINTAINER_URL", value: pkg.MaintainerURL},
	}
}

// PackageEnv renders package.env, which the package scripts load (ADR 0026 section 6): ID,
// DISPLAY_NAME, ENV_PREFIX and PORT.
func (pkg Package) PackageEnv() (string, error) {
	if err := pkg.Validate(); err != nil {
		return "", fmt.Errorf("render package.env: %w", err)
	}

	return renderEnv([]envAssignment{
		{name: "ID", value: pkg.ID},
		{name: "DISPLAY_NAME", value: pkg.DisplayName},
		{name: "ENV_PREFIX", value: pkg.EnvPrefix},
		{name: "PORT", value: strconv.Itoa(pkg.Port)},
	})
}
