package spkmeta_test

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/magpeek/mivo/packaging/synology/internal/spkmeta"
)

func TestMapVersionFollowsOD_SPK_5(t *testing.T) {
	t.Parallel()

	tests := []struct {
		server   string
		revision int
		wantDSM  string
		wantBeta bool
	}{
		{server: "0.1.0-alpha.2", revision: 0, wantDSM: "0.1.0-1020", wantBeta: true},
		{server: "0.1.0-alpha.2", revision: 1, wantDSM: "0.1.0-1021", wantBeta: true},
		{server: "0.1.0-alpha.3", revision: 0, wantDSM: "0.1.0-1030", wantBeta: true},
		{server: "0.1.1-alpha.1", revision: 0, wantDSM: "0.1.1-1010", wantBeta: true},
		{server: "0.1.0-beta.1", revision: 0, wantDSM: "0.1.0-2010", wantBeta: true},
		{server: "0.1.0-rc.3", revision: 0, wantDSM: "0.1.0-3030", wantBeta: true},
		{server: "0.1.0", revision: 0, wantDSM: "0.1.0-9000", wantBeta: false},
		{server: "0.1.0", revision: 9, wantDSM: "0.1.0-9009", wantBeta: false},
		{server: "0.0.0-dev", revision: 0, wantDSM: "0.0.0-0000", wantBeta: true},
		{server: "0.0.0-dev", revision: 3, wantDSM: "0.0.0-0003", wantBeta: true},
		{server: "1.2.3-alpha.0", revision: 0, wantDSM: "1.2.3-1000", wantBeta: true},
		{server: "1.2.3-rc.99", revision: 9, wantDSM: "1.2.3-3999", wantBeta: true},
		{server: "10.20.30", revision: 0, wantDSM: "10.20.30-9000", wantBeta: false},
		{server: "2147483647.0.0", revision: 0, wantDSM: "2147483647.0.0-9000", wantBeta: false},
	}

	for _, test := range tests {
		name := test.server + "/rev" + strconv.Itoa(test.revision)
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := spkmeta.MapVersion(test.server, test.revision)
			if err != nil {
				t.Fatalf("MapVersion(%q, %d): %v", test.server, test.revision, err)
			}

			want := spkmeta.Version{Server: test.server, DSM: test.wantDSM, Beta: test.wantBeta}
			if got != want {
				t.Errorf("MapVersion(%q, %d) = %+v, want %+v", test.server, test.revision, got, want)
			}
		})
	}
}

func TestBetaFlagSpellsYesOrNo(t *testing.T) {
	t.Parallel()

	if got := (spkmeta.Version{Beta: true}).BetaFlag(); got != "yes" {
		t.Errorf("BetaFlag() of a pre-release = %q, want yes", got)
	}

	if got := (spkmeta.Version{Beta: false}).BetaFlag(); got != "no" {
		t.Errorf("BetaFlag() of a final release = %q, want no", got)
	}
}

func TestMapVersionRejectsInvalidVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		server   string
		revision int
		wantText string
	}{
		{name: "leading v", server: "v0.1.0", wantText: "leading v"},
		{name: "leading v pre-release", server: "v0.1.0-alpha.2", wantText: "leading v"},
		{name: "build metadata", server: "0.1.0+build.5", wantText: "build metadata"},
		{name: "build metadata pre-release", server: "0.1.0-alpha.2+abc", wantText: "build metadata"},
		{name: "unknown identifier", server: "0.1.0-preview.1", wantText: "pre-release identifier"},
		{name: "upper-case identifier", server: "0.1.0-ALPHA.1", wantText: "pre-release identifier"},
		{name: "dev with number", server: "0.0.0-dev.1", wantText: "pre-release identifier"},
		{name: "dev on a real version", server: "0.1.0-dev", wantText: "0.0.0-dev"},
		{name: "identifier without number", server: "0.1.0-alpha", wantText: "pre-release number"},
		{name: "empty pre-release", server: "0.1.0-", wantText: "pre-release identifier"},
		{name: "pre-release number above 99", server: "0.1.0-alpha.100", wantText: "pre-release number"},
		{name: "pre-release number not numeric", server: "0.1.0-beta.x", wantText: "pre-release number"},
		{name: "pre-release number leading zero", server: "0.1.0-rc.03", wantText: "leading zero"},
		{name: "pre-release with extra part", server: "0.1.0-alpha.1.2", wantText: "pre-release number"},
		{name: "revision below 0", server: "0.1.0", revision: -1, wantText: "revision"},
		{name: "revision above 9", server: "0.1.0", revision: 10, wantText: "revision"},
		{name: "non-numeric core", server: "0.x.0", wantText: "core"},
		{name: "two core parts", server: "0.1", wantText: "core"},
		{name: "four core parts", server: "0.1.0.0", wantText: "core"},
		{name: "empty core part", server: "0..0", wantText: "core"},
		{name: "empty version", server: "", wantText: "core"},
		{name: "signed core part", server: "+1.0.0", wantText: "build metadata"},
		{name: "negative core part", server: "0.-1.0", wantText: "core"},
		{name: "core leading zero", server: "0.01.0", wantText: "leading zero"},
		{name: "major above 2^31-1", server: "2147483648.0.0", wantText: "2147483647"},
		{name: "patch far above 2^31-1", server: "0.0.99999999999999999999", wantText: "2147483647"},
		{name: "white space", server: " 0.1.0", wantText: "core"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := spkmeta.MapVersion(test.server, test.revision)
			if !errors.Is(err, spkmeta.ErrInvalidVersion) {
				t.Fatalf("MapVersion(%q, %d) = %+v, %v; want ErrInvalidVersion", test.server, test.revision, got, err)
			}

			if !strings.Contains(err.Error(), test.wantText) {
				t.Errorf("error %q does not mention %q", err, test.wantText)
			}
		})
	}
}

// TestMapVersionIsMonotonic checks the mapping against the comparison rule ADR 0026 Notes
// assume for DSM: the dot-separated feature numbers numerically from the left, then the build
// number. The server versions are listed in SemVer precedence order.
func TestMapVersionIsMonotonic(t *testing.T) {
	t.Parallel()

	sortedServerVersions := []string{
		"0.0.0-dev",
		"0.0.1-alpha.1",
		"0.1.0-alpha.0",
		"0.1.0-alpha.2",
		"0.1.0-alpha.3",
		"0.1.0-alpha.10",
		"0.1.0-alpha.99",
		"0.1.0-beta.1",
		"0.1.0-beta.2",
		"0.1.0-rc.1",
		"0.1.0-rc.3",
		"0.1.0",
		"0.1.1-alpha.1",
		"0.1.1",
		"0.2.0-beta.1",
		"0.10.0",
		"1.0.0-rc.1",
		"1.0.0",
		"1.0.10",
		"10.0.0",
	}

	var previous dsmVersion

	for index, server := range sortedServerVersions {
		for revision := range 10 {
			mapped, err := spkmeta.MapVersion(server, revision)
			if err != nil {
				t.Fatalf("MapVersion(%q, %d): %v", server, revision, err)
			}

			current := parseDSMVersion(t, mapped.DSM)
			if (index > 0 || revision > 0) && !previous.less(current) {
				t.Errorf("%s (rev %d) maps to %s, not above the previous %v", server, revision, mapped.DSM, previous)
			}

			previous = current
		}
	}
}

// dsmVersion is a DSM package version "major.minor.patch-build" split into numbers.
type dsmVersion [4]int64

func (version dsmVersion) less(other dsmVersion) bool {
	for index := range version {
		if version[index] != other[index] {
			return version[index] < other[index]
		}
	}

	return false
}

func parseDSMVersion(t *testing.T, text string) dsmVersion {
	t.Helper()

	feature, build, found := strings.Cut(text, "-")
	parts := strings.Split(feature, ".")

	if !found || len(parts) != 3 {
		t.Fatalf("DSM version %q is not major.minor.patch-build", text)
	}

	var version dsmVersion

	for index, part := range append(parts, build) {
		number, err := strconv.ParseInt(part, 10, 32)
		if err != nil || number < 0 {
			t.Fatalf("DSM version %q: part %q is not a number in 0..2^31-1: %v", text, part, err)
		}

		version[index] = number
	}

	return version
}

func (version dsmVersion) String() string {
	return fmt.Sprintf("%d.%d.%d-%d", version[0], version[1], version[2], version[3])
}
