package spkmeta_test

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/magpeek/mivo/packaging/synology/internal/spkmeta"
)

// decodedDesktopConfig parses the application config into its ".url" entries.
func decodedDesktopConfig(t *testing.T, pkg spkmeta.Package) map[string]map[string]any {
	t.Helper()

	content, err := pkg.DesktopAppConfig()
	if err != nil {
		t.Fatalf("DesktopAppConfig() error = %v", err)
	}

	if !strings.HasSuffix(string(content), "}\n") {
		t.Errorf("config does not end with a newline: %q", content)
	}

	var config map[string]map[string]map[string]any
	if err := json.Unmarshal(content, &config); err != nil {
		t.Fatalf("config is not JSON: %v\n%s", err, content)
	}

	if len(config) != 1 || config[".url"] == nil {
		t.Fatalf("config has keys %v, want only .url", keysOf(config))
	}

	return config[".url"]
}

func keysOf[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	return keys
}

func TestDesktopAppConfigOpensTheWebAppForEveryUser(t *testing.T) {
	t.Parallel()

	entries := decodedDesktopConfig(t, demoPackage())

	entry, found := entries["com.examplemaintainer.demo"]
	if !found || len(entries) != 1 {
		t.Fatalf("entries %v, want exactly com.examplemaintainer.demo", keysOf(entries))
	}

	want := map[string]any{
		"type":                  "url",
		"title":                 "Demo Server",
		"desc":                  "Opens the web app of the media server",
		"icon":                  "images/demo_{0}.png",
		"protocol":              "http",
		"port":                  "9999",
		"url":                   "/",
		"allUsers":              true,
		"grantPrivilege":        "all",
		"advanceGrantPrivilege": true,
	}

	if len(entry) != len(want) {
		t.Errorf("entry has keys %v, want %v", keysOf(entry), keysOf(want))
	}

	for key, value := range want {
		if entry[key] != value {
			t.Errorf("%s = %#v, want %#v", key, entry[key], value)
		}
	}
}

// INFO dsmappname must equal the unique property name of the entry; without it DSM shows no
// main menu entry (DSM-NOTES section 16, SPK-12 owner test run 2).
func TestDesktopAppNameIsTheKeyOfTheConfigEntry(t *testing.T) {
	t.Parallel()

	pkg := demoPackage()

	name, err := pkg.DesktopAppName()
	if err != nil {
		t.Fatalf("DesktopAppName() error = %v", err)
	}

	if name != "com.examplemaintainer.demo" {
		t.Errorf("DesktopAppName() = %q, want com.examplemaintainer.demo", name)
	}

	if _, found := decodedDesktopConfig(t, pkg)[name]; !found {
		t.Errorf("config has no entry %q", name)
	}
}

func TestDesktopAppNameRefusesAnInvalidPackage(t *testing.T) {
	t.Parallel()

	pkg := demoPackage()
	pkg.Maintainer = "---"

	if _, err := pkg.DesktopAppName(); !errors.Is(err, spkmeta.ErrInvalidValue) {
		t.Errorf("DesktopAppName() error = %v, want %v", err, spkmeta.ErrInvalidValue)
	}
}

// The title is the display name as it is, also with characters JSON must escape.
func TestDesktopAppConfigCarriesTheDisplayName(t *testing.T) {
	t.Parallel()

	pkg := demoPackage()
	pkg.DisplayName = "Médiathèque <Demo> & Co"

	entry := decodedDesktopConfig(t, pkg)["com.examplemaintainer.demo"]
	if entry["title"] != pkg.DisplayName {
		t.Errorf("title = %#v, want %q", entry["title"], pkg.DisplayName)
	}
}

func TestDesktopAppConfigRefusesAnInvalidPackage(t *testing.T) {
	t.Parallel()

	pkg := demoPackage()
	pkg.ID = "Demo"

	if _, err := pkg.DesktopAppConfig(); !errors.Is(err, spkmeta.ErrInvalidValue) {
		t.Errorf("DesktopAppConfig() error = %v, want %v", err, spkmeta.ErrInvalidValue)
	}
}

// The guide replaces {0} of "icon" with 16, 24, 32, 48, 64, 72 and 256.
func TestDesktopIconsFollowTheIconTemplate(t *testing.T) {
	t.Parallel()

	want := []int{16, 24, 32, 48, 64, 72, 256}
	if sizes := spkmeta.DesktopIconSizes(); !slices.Equal(sizes, want) {
		t.Errorf("DesktopIconSizes() = %v, want %v", sizes, want)
	}

	pkg := demoPackage()
	if path := pkg.DesktopIconPath(72); path != "images/demo_72.png" {
		t.Errorf("DesktopIconPath(72) = %q, want images/demo_72.png", path)
	}

	sizes := spkmeta.DesktopIconSizes()
	sizes[0] = 1

	if spkmeta.DesktopIconSizes()[0] != want[0] {
		t.Error("DesktopIconSizes() returns the package's own slice")
	}
}
