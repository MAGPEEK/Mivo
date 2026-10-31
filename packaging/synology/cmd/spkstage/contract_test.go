package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/magpeek/mivo/packaging/synology/internal/spkmeta"
)

// fixturePackage holds the brand of packaging/synology/test/fixtures/package.env, the file the
// script tests (bats) load into common.sh under four shells.
func fixturePackage() spkmeta.Package {
	pkg := spkmeta.DefaultPackage()
	pkg.ID = "demo"
	pkg.DisplayName = "Demo Media Server"
	pkg.EnvPrefix = "DEMO"
	pkg.Port = 9999

	return pkg
}

// TestScriptFixtureIsSpkmetaOutput ties the script tests to the one source of package.env:
// the assignments of the bats fixture must be exactly what spkmeta renders for its brand, so
// common.sh is tested against the real format.
func TestScriptFixtureIsSpkmetaOutput(t *testing.T) {
	t.Parallel()

	want, err := fixturePackage().PackageEnv()
	if err != nil {
		t.Fatalf("PackageEnv: %v", err)
	}

	fixture := filepath.Join(repoRoot(t), "packaging", "synology", "test", "fixtures", "package.env")

	content, err := os.ReadFile(fixture) //nolint:gosec // fixed path in the repository
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var assignments strings.Builder

	for line := range strings.SplitSeq(string(content), "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			assignments.WriteString(line + "\n")
		}
	}

	if assignments.String() != want {
		t.Errorf("fixture assignments:\n%s\nspkmeta renders:\n%s", assignments.String(), want)
	}
}

// TestCommonShLoadsTheStagedPackageEnv sources the staged common.sh and package.env with the
// system's sh, as the package scripts do on DSM. It is skipped where no sh exists (Windows
// without Git Bash on the PATH); the bats tests cover the same contract under four shells.
func TestCommonShLoadsTheStagedPackageEnv(t *testing.T) {
	t.Parallel()

	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on the PATH")
	}

	opts := testOptions(t, "x86_64", writeInputs(t, "x86_64", fakeSourceSelected))
	if _, err := stage(opts, spkmeta.DefaultPackage(), os.Stderr); err != nil {
		t.Fatalf("stage: %v", err)
	}

	scripts := filepath.ToSlash(filepath.Join(opts.out, scriptsDir))
	command := exec.CommandContext(context.Background(), shell, "-c", //nolint:gosec // sh from the PATH runs a fixed script
		`PKG_SCRIPT_DIR=$1; . "$PKG_SCRIPT_DIR/common.sh" && pkg_load_package_env && printf '%s|%s|%s|%s' "$ID" "$DISPLAY_NAME" "$ENV_PREFIX" "$PORT"`,
		"sh", scripts)

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("sh: %v: %s", err, output)
	}

	pkg := spkmeta.DefaultPackage()
	if want := pkg.ID + "|" + pkg.DisplayName + "|" + pkg.EnvPrefix + "|" + strconv.Itoa(pkg.Port); string(output) != want {
		t.Errorf("common.sh loaded %q, want %q", output, want)
	}
}
