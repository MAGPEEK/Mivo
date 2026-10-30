package archtest

import (
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// TestDepguardDeniesEveryStandardPackageOfTheMatrix guards against toolchain upgrades: a new
// standard library package below a denied prefix (e.g. a future os/... package) is caught
// by archtest, but depguard only denies exact entries, so .golangci.yml must list it too.
func TestDepguardDeniesEveryStandardPackageOfTheMatrix(t *testing.T) {
	t.Parallel()

	moduleRoot := moduleRootOfTest(t)
	standardPackages := listStandardPackages(t, moduleRoot)

	denies := depguardDenies(readModuleFile(t, moduleRoot, golangciConfigFile), layerMatrix())

	for _, owner := range layerMatrix() {
		if !owner.standardLibraryOnly {
			continue
		}

		if len(denies[owner.ruleName()]) == 0 {
			t.Errorf("%s has no deny entries for depguard rule %q", golangciConfigFile, owner.ruleName())

			continue
		}

		for _, missing := range missingStandardDenies(owner, standardPackages, denies[owner.ruleName()]) {
			t.Errorf("depguard rule %q in %s lacks `- pkg: %s$` (denied by archtest for %s)",
				owner.ruleName(), golangciConfigFile, missing, owner.dir)
		}
	}
}

func TestDepguardDeniesCollectsLayerRulesOnly(t *testing.T) {
	t.Parallel()

	config := `linters:
  settings:
    depguard:
      rules:
        domain:
          deny:
            - pkg: os$
              desc: pure
            - pkg: "net/http$"
        other:
          deny:
            - pkg: ignored$
        app:
          files:
            - "**/internal/app/**"
          deny:
            - pkg: 'os/exec$'
    forbidigo:
      forbid:
        - pattern: ^slog\.Info$
          pkg: ^log/slog$
        - pkg: outside-rule$
`

	got := depguardDenies(config, layerMatrix())

	want := map[string][]string{"domain": {"os$", "net/http$"}, "app": {"os/exec$"}}
	if len(got) != len(want) {
		t.Fatalf("depguardDenies = %q, want %q", got, want)
	}

	for rule, wantDenies := range want {
		if !slices.Equal(got[rule], wantDenies) {
			t.Errorf("denies of %q = %q, want %q", rule, got[rule], wantDenies)
		}
	}
}

func TestMissingStandardDeniesSkipsAllowedAndInternalPackages(t *testing.T) {
	t.Parallel()

	domain := layer{dir: "internal/domain", denied: []importRule{prefix("os"), exactly("net"), exactly("C")}}
	standardPackages := []string{"net", "net/url", "os", "os/exec", "os/exec/internal/fdtest", "os/signal", "strings"}
	configuredDenies := []string{"os$", "os/exec$"}

	got := missingStandardDenies(domain, standardPackages, configuredDenies)

	if want := []string{"net", "os/signal"}; !slices.Equal(got, want) {
		t.Errorf("missingStandardDenies = %q, want %q", got, want)
	}
}

func moduleRootOfTest(t *testing.T) string {
	t.Helper()

	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}

	moduleRoot, err := findModuleRoot(workingDir)
	if err != nil {
		t.Fatalf("find module root: %v", err)
	}

	return moduleRoot
}

// readModuleFile reads name through an os.Root, so it cannot escape the module root.
func readModuleFile(t *testing.T, moduleRoot, name string) string {
	t.Helper()

	root, err := os.OpenRoot(moduleRoot)
	if err != nil {
		t.Fatalf("open module root %s: %v", moduleRoot, err)
	}
	content, readErr := root.ReadFile(name)
	if closeErr := root.Close(); closeErr != nil {
		t.Fatalf("close module root %s: %v", moduleRoot, closeErr)
	}

	if readErr != nil {
		t.Fatalf("read %s: %v", name, readErr)
	}

	return string(content)
}

// listStandardPackages runs `go list std`; go test puts its own GOROOT/bin first in PATH,
// so the list matches the toolchain running the tests.
func listStandardPackages(t *testing.T, moduleRoot string) []string {
	t.Helper()

	command := exec.CommandContext(t.Context(), "go", "list", "std")
	command.Dir = moduleRoot

	output, err := command.Output()
	if err != nil {
		t.Fatalf("go list std: %v", err)
	}

	standardPackages := strings.Fields(string(output))
	if len(standardPackages) == 0 {
		t.Fatal("go list std returned no packages")
	}

	return standardPackages
}
