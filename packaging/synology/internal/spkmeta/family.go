package spkmeta

import (
	"fmt"
	"strings"
)

// Family is one DSM architecture family with one SPK (ADR 0026 section 3 and Notes).
type Family struct {
	// Name is the family: "x86_64", "armv8" or "armv7".
	Name string
	// Arch is INFO's "arch" value. Its first word is the family, which INFO.sh checks against
	// the toolkit's pkg_get_platform_family; further words are platforms DSM keeps outside the
	// family.
	Arch string
	// ToolkitPlatform is the pkgscripts-ng platform environment that packs the SPK.
	ToolkitPlatform string
	// GOARCH is the Go target architecture of the static server binary.
	GOARCH string
	// GOARM is the Go ARM version, empty for other architectures.
	GOARM string
	// DockerPlatform is the platform of the emulated tests, for example "linux/arm/v7".
	DockerPlatform string
	// FFmpegKey selects the FFmpeg build of the family (sources.lock entry and the target of
	// the own FFmpeg build).
	FFmpegKey string
}

// Families returns the families in release order: x86_64, armv8, armv7. Each call returns a
// new slice.
//
// The armv7 SPK also names armada38x and monaco, which DSM keeps outside the armv7 family
// (orchestrator decision, ADR 0026 Notes); its binaries target ARMv7 hard-float with VFPv3
// and no NEON assumption, which GOARM=7 meets.
func Families() []Family {
	return []Family{
		{
			Name: "x86_64", Arch: "x86_64", ToolkitPlatform: "v1000",
			GOARCH: "amd64", DockerPlatform: "linux/amd64", FFmpegKey: "x86_64",
		},
		{
			Name: "armv8", Arch: "armv8", ToolkitPlatform: "rtd1619b",
			GOARCH: "arm64", DockerPlatform: "linux/arm64", FFmpegKey: "armv8",
		},
		{
			Name: "armv7", Arch: "armv7 armada38x monaco", ToolkitPlatform: "alpine",
			GOARCH: "arm", GOARM: "7", DockerPlatform: "linux/arm/v7", FFmpegKey: "armv7",
		},
	}
}

// LookupFamily returns the family with the given name.
func LookupFamily(name string) (Family, error) {
	names := make([]string, 0, len(Families()))

	for _, family := range Families() {
		if family.Name == name {
			return family, nil
		}

		names = append(names, family.Name)
	}

	return Family{}, fmt.Errorf("family %q (want one of %s): %w", name, strings.Join(names, ", "), ErrUnknownFamily)
}
