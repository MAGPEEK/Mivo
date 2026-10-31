package spkmeta_test

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/platform/brand"
	"github.com/magpeek/mivo/internal/platform/config"
	"github.com/magpeek/mivo/packaging/synology/internal/spkmeta"
)

// demoPackage is a brand that differs from the real one, so the tests prove that every value
// comes from the Package and nothing is hard-coded.
func demoPackage() spkmeta.Package {
	return spkmeta.Package{
		ID:                "demo",
		DisplayName:       "Demo Server",
		EnvPrefix:         "DEMO",
		Description:       "Placeholder package of the tests.",
		DescriptionGerman: "Platzhalterpaket für die Tests.",
		Maintainer:        "Example Maintainer",
		MaintainerURL:     "https://example.invalid/demo",
		Port:              9999,
		OSMinVersion:      "7.2-64570",
	}
}

func demoBuild(t *testing.T, version, family string) spkmeta.Build {
	t.Helper()

	build, err := spkmeta.NewBuild(demoPackage(), spkmeta.BuildOptions{ServerVersion: version, Family: family})
	if err != nil {
		t.Fatalf("NewBuild(%s, %s): %v", version, family, err)
	}

	return build
}

func TestBuildEnvRendersEveryFieldINFOShNeeds(t *testing.T) {
	t.Parallel()

	got, err := demoBuild(t, "0.1.0-alpha.2", "x86_64").BuildEnv()
	if err != nil {
		t.Fatalf("BuildEnv: %v", err)
	}

	want := "ID='demo'\n" +
		"DISPLAY_NAME='Demo Server'\n" +
		"DESCRIPTION='Placeholder package of the tests.'\n" +
		"DESCRIPTION_GER='Platzhalterpaket für die Tests.'\n" +
		"MAINTAINER='Example Maintainer'\n" +
		"MAINTAINER_URL='https://example.invalid/demo'\n" +
		"VERSION='0.1.0-alpha.2'\n" +
		"DSM_VERSION='0.1.0-1020'\n" +
		"BETA='yes'\n" +
		"OS_MIN_VER='7.2-64570'\n" +
		"ARCH='x86_64'\n" +
		"PORT='9999'\n"
	if got != want {
		t.Errorf("BuildEnv() =\n%s\nwant\n%s", got, want)
	}
}

func TestBuildEnvOfAFinalArmv7Release(t *testing.T) {
	t.Parallel()

	build, err := spkmeta.NewBuild(demoPackage(), spkmeta.BuildOptions{
		ServerVersion: "0.1.0", PackageRevision: 2, Family: "armv7",
	})
	if err != nil {
		t.Fatalf("NewBuild: %v", err)
	}

	got, err := build.BuildEnv()
	if err != nil {
		t.Fatalf("BuildEnv: %v", err)
	}

	for _, line := range []string{"VERSION='0.1.0'", "DSM_VERSION='0.1.0-9002'", "BETA='no'", "ARCH='armv7 armada38x monaco'"} {
		if !strings.Contains(got, line+"\n") {
			t.Errorf("BuildEnv() lacks %s:\n%s", line, got)
		}
	}
}

func TestPackageEnvRendersTheScriptValues(t *testing.T) {
	t.Parallel()

	got, err := demoPackage().PackageEnv()
	if err != nil {
		t.Fatalf("PackageEnv: %v", err)
	}

	want := "ID='demo'\nDISPLAY_NAME='Demo Server'\nENV_PREFIX='DEMO'\nPORT='9999'\n"
	if got != want {
		t.Errorf("PackageEnv() =\n%s\nwant\n%s", got, want)
	}
}

// The grammar contract shared with the toolkit scripts: every line is one single-quoted
// assignment, so the files can be sourced and also parsed without a shell.
func TestEnvFilesFollowTheGrammar(t *testing.T) {
	t.Parallel()

	grammar := regexp.MustCompile(`^[A-Z_][A-Z0-9_]*='[^']*'$`)

	for _, family := range spkmeta.Families() {
		build, err := spkmeta.NewBuild(spkmeta.DefaultPackage(), spkmeta.BuildOptions{
			ServerVersion: "0.1.0-alpha.2", Family: family.Name,
		})
		if err != nil {
			t.Fatalf("NewBuild(%s): %v", family.Name, err)
		}

		buildEnv, buildErr := build.BuildEnv()
		packageEnv, packageErr := build.Package.PackageEnv()

		if buildErr != nil || packageErr != nil {
			t.Fatalf("render %s: %v, %v", family.Name, buildErr, packageErr)
		}

		assertGrammar(t, grammar, "build.env "+family.Name, buildEnv)
		assertGrammar(t, grammar, "package.env "+family.Name, packageEnv)
	}
}

func assertGrammar(t *testing.T, grammar *regexp.Regexp, fileName, content string) {
	t.Helper()

	if !strings.HasSuffix(content, "\n") {
		t.Errorf("%s does not end with a newline", fileName)
	}

	for line := range strings.Lines(content) {
		if !grammar.MatchString(strings.TrimSuffix(line, "\n")) {
			t.Errorf("%s line %q breaks the grammar", fileName, line)
		}
	}
}

func TestDefaultPackageComesFromTheBrand(t *testing.T) {
	t.Parallel()

	got := spkmeta.DefaultPackage()

	want := spkmeta.Package{
		ID:                brand.ID,
		DisplayName:       brand.DisplayName,
		EnvPrefix:         brand.EnvPrefix,
		Description:       got.Description,
		DescriptionGerman: got.DescriptionGerman,
		Maintainer:        "MAGPEEK",
		MaintainerURL:     brand.RepositoryURL,
		Port:              config.DefaultPort,
		OSMinVersion:      "7.2-64570",
	}
	if got != want {
		t.Errorf("DefaultPackage() = %+v, want %+v", got, want)
	}

	if err := got.Validate(); err != nil {
		t.Errorf("DefaultPackage().Validate(): %v", err)
	}
}

// ADR 0026: the package is the media server running on the NAS, not a player app, and the
// German description is real German text.
func TestDefaultDescriptionsDescribeTheServer(t *testing.T) {
	t.Parallel()

	pkg := spkmeta.DefaultPackage()

	if !containsAll(strings.ToLower(pkg.Description), "media server", "nas", "browser") {
		t.Errorf("Description %q does not describe the media server on the NAS", pkg.Description)
	}

	if !containsAll(strings.ToLower(pkg.DescriptionGerman), "medienserver", "nas", "browser", "ä") {
		t.Errorf("DescriptionGerman %q is not the German server description", pkg.DescriptionGerman)
	}

	for _, description := range []string{pkg.Description, pkg.DescriptionGerman} {
		if strings.Contains(strings.ToLower(description), strings.ToLower(brand.DisplayName)) {
			t.Errorf("description %q names the product", description)
		}
	}
}

func TestBuildEnvAcceptsUmlautsInTexts(t *testing.T) {
	t.Parallel()

	pkg := demoPackage()
	pkg.DisplayName = "Medienzentrale Öl"
	pkg.Maintainer = "Jürgen Müller"

	build, err := spkmeta.NewBuild(pkg, spkmeta.BuildOptions{ServerVersion: "0.1.0", Family: "armv8"})
	if err != nil {
		t.Fatalf("NewBuild: %v", err)
	}

	got, err := build.BuildEnv()
	if err != nil {
		t.Fatalf("BuildEnv: %v", err)
	}

	if !strings.Contains(got, "DISPLAY_NAME='Medienzentrale Öl'\n") || !strings.Contains(got, "MAINTAINER='Jürgen Müller'\n") {
		t.Errorf("umlauts lost:\n%s", got)
	}
}

func TestPackageTextsRejectUnsafeCharacters(t *testing.T) {
	t.Parallel()

	unsafe := infoUnsafeValues()
	for name, value := range infoMainUnsafeValues() {
		unsafe[name] = value
	}

	unsafe["single quote"] = "Demo's Server"
	unsafe["carriage return"] = "Demo\rServer"
	unsafe["escape"] = "Demo\x1bServer"
	unsafe["delete"] = "Demo\x7fServer"

	setters := map[string]func(*spkmeta.Package, string){
		"DISPLAY_NAME":    func(pkg *spkmeta.Package, value string) { pkg.DisplayName = value },
		"DESCRIPTION":     func(pkg *spkmeta.Package, value string) { pkg.Description = value },
		"DESCRIPTION_GER": func(pkg *spkmeta.Package, value string) { pkg.DescriptionGerman = value },
		"MAINTAINER":      func(pkg *spkmeta.Package, value string) { pkg.Maintainer = value },
		"MAINTAINER_URL":  func(pkg *spkmeta.Package, value string) { pkg.MaintainerURL = value },
	}

	for field, set := range setters {
		for character, value := range unsafe {
			t.Run(field+"/"+character, func(t *testing.T) {
				t.Parallel()

				pkg := demoPackage()
				set(&pkg, value)
				assertPackageRejected(t, pkg, spkmeta.ErrUnsafeValue, field)
			})
		}
	}
}

func TestPackageValidateRejectsMalformedValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		field string
		edit  func(*spkmeta.Package)
	}{
		{name: "empty ID", field: "ID", edit: func(pkg *spkmeta.Package) { pkg.ID = "" }},
		{name: "upper-case ID", field: "ID", edit: func(pkg *spkmeta.Package) { pkg.ID = "Demo" }},
		{name: "underscore in ID", field: "ID", edit: func(pkg *spkmeta.Package) { pkg.ID = "de_mo" }},
		{name: "slash in ID", field: "ID", edit: func(pkg *spkmeta.Package) { pkg.ID = "de/mo" }},
		{name: "ID starts with dash", field: "ID", edit: func(pkg *spkmeta.Package) { pkg.ID = "-demo" }},
		{name: "empty prefix", field: "ENV_PREFIX", edit: func(pkg *spkmeta.Package) { pkg.EnvPrefix = "" }},
		{name: "lower-case prefix", field: "ENV_PREFIX", edit: func(pkg *spkmeta.Package) { pkg.EnvPrefix = "demo" }},
		{name: "prefix starts with digit", field: "ENV_PREFIX", edit: func(pkg *spkmeta.Package) { pkg.EnvPrefix = "1DEMO" }},
		{name: "dash in prefix", field: "ENV_PREFIX", edit: func(pkg *spkmeta.Package) { pkg.EnvPrefix = "DE-MO" }},
		{name: "port 0", field: "PORT", edit: func(pkg *spkmeta.Package) { pkg.Port = 0 }},
		{name: "port above 65535", field: "PORT", edit: func(pkg *spkmeta.Package) { pkg.Port = 65536 }},
		{name: "empty display name", field: "DISPLAY_NAME", edit: func(pkg *spkmeta.Package) { pkg.DisplayName = "" }},
		{name: "os_min_ver without build", field: "OS_MIN_VER", edit: func(pkg *spkmeta.Package) { pkg.OSMinVersion = "7.2" }},
		{name: "os_min_ver text build", field: "OS_MIN_VER", edit: func(pkg *spkmeta.Package) { pkg.OSMinVersion = "7.2-abc" }},
		{name: "empty os_min_ver", field: "OS_MIN_VER", edit: func(pkg *spkmeta.Package) { pkg.OSMinVersion = "" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			pkg := demoPackage()
			test.edit(&pkg)
			assertPackageRejected(t, pkg, spkmeta.ErrInvalidValue, test.field)
		})
	}
}

func TestPackageValidateAcceptsPortBounds(t *testing.T) {
	t.Parallel()

	for _, port := range []int{1, 65535} {
		pkg := demoPackage()
		pkg.Port = port

		if err := pkg.Validate(); err != nil {
			t.Errorf("port %d rejected: %v", port, err)
		}
	}
}

func TestNewBuildRejectsBadInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options spkmeta.BuildOptions
		want    error
	}{
		{name: "unknown family", options: spkmeta.BuildOptions{ServerVersion: "0.1.0", Family: "mips"}, want: spkmeta.ErrUnknownFamily},
		{name: "invalid version", options: spkmeta.BuildOptions{ServerVersion: "v0.1.0", Family: "x86_64"}, want: spkmeta.ErrInvalidVersion},
		{name: "invalid revision", options: spkmeta.BuildOptions{ServerVersion: "0.1.0", PackageRevision: 10, Family: "x86_64"}, want: spkmeta.ErrInvalidVersion},
	}

	for _, test := range tests {
		if _, err := spkmeta.NewBuild(demoPackage(), test.options); !errors.Is(err, test.want) {
			t.Errorf("%s: NewBuild = %v, want %v", test.name, err, test.want)
		}
	}

	pkg := demoPackage()
	pkg.ID = "Demo"

	if _, err := spkmeta.NewBuild(pkg, spkmeta.BuildOptions{ServerVersion: "0.1.0", Family: "x86_64"}); !errors.Is(err, spkmeta.ErrInvalidValue) {
		t.Errorf("invalid package: NewBuild = %v, want ErrInvalidValue", err)
	}
}

// A Build assembled by hand (not through NewBuild) is still checked before anything is
// rendered.
func TestRenderingRevalidatesAHandMadeBuild(t *testing.T) {
	t.Parallel()

	valid := demoBuild(t, "0.1.0-beta.1", "armv8")

	tests := []struct {
		name string
		edit func(*spkmeta.Build)
		want error
	}{
		{name: "unsafe arch", edit: func(build *spkmeta.Build) { build.Family.Arch = "armv8 $x" }, want: spkmeta.ErrUnsafeValue},
		{name: "empty arch", edit: func(build *spkmeta.Build) { build.Family.Arch = "" }, want: spkmeta.ErrInvalidValue},
		{name: "unsafe DSM version", edit: func(build *spkmeta.Build) { build.Version.DSM = "0.1.0-`x`" }, want: spkmeta.ErrUnsafeValue},
		{name: "quote in server version", edit: func(build *spkmeta.Build) { build.Version.Server = "0.1.0'" }, want: spkmeta.ErrUnsafeValue},
		{name: "empty server version", edit: func(build *spkmeta.Build) { build.Version.Server = "" }, want: spkmeta.ErrInvalidValue},
		{name: "invalid package", edit: func(build *spkmeta.Build) { build.Package.Port = -1 }, want: spkmeta.ErrInvalidValue},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			build := valid
			test.edit(&build)

			if _, err := build.BuildEnv(); !errors.Is(err, test.want) {
				t.Errorf("BuildEnv() = %v, want %v", err, test.want)
			}

			if _, err := build.RenderTemplate("conf/resource", "@ID@\n"); !errors.Is(err, test.want) {
				t.Errorf("RenderTemplate() = %v, want %v", err, test.want)
			}
		})
	}
}

func assertPackageRejected(t *testing.T, pkg spkmeta.Package, want error, field string) {
	t.Helper()

	if err := pkg.Validate(); !errors.Is(err, want) || !strings.Contains(err.Error(), field) {
		t.Errorf("Validate() = %v, want %v naming %s", err, want, field)
	}

	if _, err := pkg.PackageEnv(); !errors.Is(err, want) {
		t.Errorf("PackageEnv() = %v, want %v", err, want)
	}

	build := spkmeta.Build{Package: pkg, Version: spkmeta.Version{Server: "0.1.0", DSM: "0.1.0-9000"}}
	build.Family, _ = spkmeta.LookupFamily("x86_64")

	if _, err := build.BuildEnv(); !errors.Is(err, want) {
		t.Errorf("BuildEnv() = %v, want %v", err, want)
	}
}

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}

	return true
}

func TestPortIsRenderedAsDecimal(t *testing.T) {
	t.Parallel()

	got, err := spkmeta.DefaultPackage().PackageEnv()
	if err != nil {
		t.Fatalf("PackageEnv: %v", err)
	}

	if want := "PORT='" + strconv.Itoa(config.DefaultPort) + "'\n"; !strings.Contains(got, want) {
		t.Errorf("PackageEnv() lacks %q:\n%s", want, got)
	}
}
