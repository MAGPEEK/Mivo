package spkmeta_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/magpeek/mivo/packaging/synology/internal/spkmeta"
)

func wantFamilies() []spkmeta.Family {
	return []spkmeta.Family{
		{
			Name: "x86_64", Arch: "x86_64", ToolkitPlatform: "v1000",
			GOARCH: "amd64", GOARM: "", DockerPlatform: "linux/amd64", FFmpegKey: "x86_64",
		},
		{
			Name: "armv8", Arch: "armv8", ToolkitPlatform: "rtd1619b",
			GOARCH: "arm64", GOARM: "", DockerPlatform: "linux/arm64", FFmpegKey: "armv8",
		},
		{
			Name: "armv7", Arch: "armv7 armada38x monaco", ToolkitPlatform: "alpine",
			GOARCH: "arm", GOARM: "7", DockerPlatform: "linux/arm/v7", FFmpegKey: "armv7",
		},
	}
}

func TestLookupFamilyReturnsTheTableRow(t *testing.T) {
	t.Parallel()

	for _, want := range wantFamilies() {
		t.Run(want.Name, func(t *testing.T) {
			t.Parallel()

			got, err := spkmeta.LookupFamily(want.Name)
			if err != nil {
				t.Fatalf("LookupFamily(%q): %v", want.Name, err)
			}

			if got != want {
				t.Errorf("LookupFamily(%q) = %+v, want %+v", want.Name, got, want)
			}
		})
	}
}

func TestFamiliesListsEveryFamilyInOrder(t *testing.T) {
	t.Parallel()

	got := spkmeta.Families()
	want := wantFamilies()

	if len(got) != len(want) {
		t.Fatalf("Families() has %d entries, want %d", len(got), len(want))
	}

	for index := range want {
		if got[index] != want[index] {
			t.Errorf("Families()[%d] = %+v, want %+v", index, got[index], want[index])
		}
	}
}

func TestFamiliesReturnsAFreshSlice(t *testing.T) {
	t.Parallel()

	first := spkmeta.Families()
	first[0].Arch = "changed"

	if spkmeta.Families()[0].Arch == "changed" {
		t.Error("Families() returns shared state")
	}
}

// INFO.sh checks the first word of ARCH against the toolkit's pkg_get_platform_family.
func TestArchStartsWithTheFamilyName(t *testing.T) {
	t.Parallel()

	for _, family := range spkmeta.Families() {
		if first, _, _ := strings.Cut(family.Arch, " "); first != family.Name {
			t.Errorf("family %s: first word of arch %q is %q", family.Name, family.Arch, first)
		}
	}
}

func TestLookupFamilyRejectsUnknownFamilies(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"", "amd64", "aarch64", "armv7 armada38x monaco", "X86_64", "armada38x", "v1000"} {
		got, err := spkmeta.LookupFamily(name)
		if !errors.Is(err, spkmeta.ErrUnknownFamily) {
			t.Errorf("LookupFamily(%q) = %+v, %v; want ErrUnknownFamily", name, got, err)
		}
	}
}
