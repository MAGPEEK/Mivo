package spkmeta_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/magpeek/mivo/packaging/synology/internal/spkmeta"
)

func TestRenderTemplateReplacesEveryBuildToken(t *testing.T) {
	t.Parallel()

	build := demoBuild(t, "0.1.0-rc.3", "armv7")
	template := "[@ID@]\ntitle=\"@DISPLAY_NAME@\"\n@ENV_PREFIX@_CONFIG_DIR\ndst.ports=\"@PORT@/tcp\"\n" +
		"server @VERSION@ is package @DSM_VERSION@; @ID@@PORT@\n"

	got, err := build.RenderTemplate("port_conf/package.sc", template)
	if err != nil {
		t.Fatalf("RenderTemplate: %v", err)
	}

	want := "[demo]\ntitle=\"Demo Server\"\nDEMO_CONFIG_DIR\ndst.ports=\"9999/tcp\"\n" +
		"server 0.1.0-rc.3 is package 0.1.0-3030; demo9999\n"
	if got != want {
		t.Errorf("RenderTemplate() =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderTemplateKeepsOtherSyntaxUnchanged(t *testing.T) {
	t.Parallel()

	build := demoBuild(t, "0.1.0", "x86_64")
	template := strings.Join([]string{
		`{"shares": "{{wizard_media_share}}"}`,
		`HOME="${SYNOPKG_PKGHOME}"; echo "$SYNOPKG_PKGVAR" $1 $@`,
		"mail user@example.invalid, @lower@ @Mixed@ @ID @_ID@ @@ @1A@ @ ID@",
		"no trailing newline",
	}, "\n")

	got, err := build.RenderTemplate("WIZARD_UIFILES/install_uifile", template)
	if err != nil {
		t.Fatalf("RenderTemplate: %v", err)
	}

	if got != template {
		t.Errorf("RenderTemplate() changed text without build tokens:\n%s", got)
	}
}

// A value that looks like a token is inserted literally and never expanded again.
func TestRenderTemplateDoesNotRescanInsertedValues(t *testing.T) {
	t.Parallel()

	pkg := demoPackage()
	pkg.DisplayName = "Demo @PORT@ Server"

	build, err := spkmeta.NewBuild(pkg, spkmeta.BuildOptions{ServerVersion: "0.1.0", Family: "x86_64"})
	if err != nil {
		t.Fatalf("NewBuild: %v", err)
	}

	got, err := build.RenderTemplate("conf/resource", "@DISPLAY_NAME@")
	if err != nil {
		t.Fatalf("RenderTemplate: %v", err)
	}

	if got != "Demo @PORT@ Server" {
		t.Errorf("RenderTemplate() = %q", got)
	}
}

func TestRenderTemplateRejectsUnknownTokens(t *testing.T) {
	t.Parallel()

	build := demoBuild(t, "0.1.0", "x86_64")

	tests := []struct {
		name     string
		template string
		token    string
		line     string
	}{
		{name: "unknown token", template: "@ID@\n\nvalue=@FAMILY@\n", token: "@FAMILY@", line: "line 3"},
		{name: "typo", template: "@DISPLAYNAME@", token: "@DISPLAYNAME@", line: "line 1"},
		{name: "after a known token", template: "a\n@ID@ @PORT_NUMBER@ @ID@", token: "@PORT_NUMBER@", line: "line 2"},
		{name: "digit token", template: "x\ny\nz\n@A1@", token: "@A1@", line: "line 4"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := build.RenderTemplate("scripts/postinst", test.template)
			if !errors.Is(err, spkmeta.ErrUnknownToken) {
				t.Fatalf("RenderTemplate() = %v, want ErrUnknownToken", err)
			}

			if !containsAll(err.Error(), "scripts/postinst", test.line, test.token) {
				t.Errorf("error %q does not name template, %s and %s", err, test.line, test.token)
			}
		})
	}
}

// The chroot's Python 3.4 reads SynoBuildConf/ with an ASCII locale (ADR 0026 Notes).
func TestRenderTemplateKeepsSynoBuildConfASCII(t *testing.T) {
	t.Parallel()

	pkg := demoPackage()
	pkg.DisplayName = "Demo Größe"

	build, err := spkmeta.NewBuild(pkg, spkmeta.BuildOptions{ServerVersion: "0.1.0", Family: "x86_64"})
	if err != nil {
		t.Fatalf("NewBuild: %v", err)
	}

	tests := []struct {
		name     string
		template string
		line     string
	}{
		{name: "SynoBuildConf/depends", template: "[default]\n# @DISPLAY_NAME@\n", line: "line 2"},
		{name: "SynoBuildConf/build", template: "# § in the template\n", line: "line 1"},
		{name: `SynoBuildConf\install`, template: "ok\nok\n# ü\n", line: "line 3"},
		{name: "./SynoBuildConf/install", template: "# ü\n", line: "line 1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := build.RenderTemplate(test.name, test.template)
			if !errors.Is(err, spkmeta.ErrNonASCII) {
				t.Fatalf("RenderTemplate() = %v, want ErrNonASCII", err)
			}

			if !containsAll(err.Error(), test.name, test.line) {
				t.Errorf("error %q does not name the template and %s", err, test.line)
			}
		})
	}

	got, err := build.RenderTemplate("WIZARD_UIFILES/install_uifile_ger", "@DISPLAY_NAME@ für")
	if err != nil || got != "Demo Größe für" {
		t.Errorf("non-ASCII outside SynoBuildConf/: %q, %v", got, err)
	}

	if _, err := build.RenderTemplate("SynoBuildConf/depends", "[default]\nall=\"7.2\"\n"); err != nil {
		t.Errorf("ASCII SynoBuildConf/depends rejected: %v", err)
	}
}

func TestRenderTemplateNamesTheTemplateInEveryError(t *testing.T) {
	t.Parallel()

	build := demoBuild(t, "0.1.0", "x86_64")

	_, err := build.RenderTemplate("conf/privilege", "@NOPE@")
	if err == nil || !strings.HasPrefix(err.Error(), "render conf/privilege") {
		t.Errorf("error %v does not start with the template name", err)
	}
}
