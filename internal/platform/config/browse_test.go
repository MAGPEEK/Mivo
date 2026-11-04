package config_test

import (
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/platform/config"
	"github.com/magpeek/mivo/internal/platform/paths"
)

func absRoot(t *testing.T, name string) string {
	t.Helper()

	return filepath.Join(t.TempDir(), name)
}

func quoteYAML(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func TestLoadBrowseRootsFromFileAndEnvironment(t *testing.T) {
	t.Parallel()

	a, b := absRoot(t, "a"), absRoot(t, "b")
	sep := string(filepath.ListSeparator)

	tests := []struct {
		name string
		yaml string
		vars map[string]string
		want []string
	}{
		{name: "file", yaml: "browse:\n  roots:\n    - " + quoteYAML(a) + "\n    - " + quoteYAML(b) + "\n", want: []string{a, b}},
		{name: "empty file list", yaml: "browse:\n  roots: []\n", want: []string{}},
		{
			name: "environment replaces the file", yaml: "browse:\n  roots:\n    - " + quoteYAML(a) + "\n",
			vars: map[string]string{env("BROWSE_ROOTS"): b}, want: []string{b},
		},
		{name: "empty environment means no roots", vars: map[string]string{env("BROWSE_ROOTS"): ""}, want: []string{}},
		{name: "path list", vars: map[string]string{env("BROWSE_ROOTS"): a + sep + b}, want: []string{a, b}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := newFixture(t, tt.yaml, tt.vars).load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}

			if !reflect.DeepEqual(got.Browse.Roots, tt.want) {
				t.Errorf("Browse.Roots = %v, want %v", got.Browse.Roots, tt.want)
			}
		})
	}
}

func TestLoadBrowseRootsRelativeResolvesAgainstConfigDir(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "", map[string]string{env("BROWSE_ROOTS"): "media"})

	got, err := f.load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if want := []string{filepath.Join(f.configDir, "media")}; !slices.Equal(got.Browse.Roots, want) {
		t.Errorf("Browse.Roots = %v, want %v", got.Browse.Roots, want)
	}
}

func TestLoadBrowseRootsInvalid(t *testing.T) {
	t.Parallel()

	a := absRoot(t, "a")
	sep := string(filepath.ListSeparator)

	tests := []struct {
		name    string
		yaml    string
		vars    map[string]string
		wantErr string
	}{
		{name: "home shorthand", vars: map[string]string{env("BROWSE_ROOTS"): "~/x"}, wantErr: env("BROWSE_ROOTS")},
		{name: "duplicate", vars: map[string]string{env("BROWSE_ROOTS"): a + sep + a}, wantErr: "duplicate"},
		{
			name:    "duplicate after cleaning",
			vars:    map[string]string{env("BROWSE_ROOTS"): a + sep + a + string(filepath.Separator) + "."},
			wantErr: "duplicate",
		},
		{name: "file duplicate", yaml: "browse:\n  roots: [" + quoteYAML(a) + ", " + quoteYAML(a) + "]\n", wantErr: "duplicate"},
		{name: "empty env item", vars: map[string]string{env("BROWSE_ROOTS"): a + sep + sep + a + "b"}, wantErr: env("BROWSE_ROOTS")},
		{name: "empty file item", yaml: "browse:\n  roots: ['']\n", wantErr: "browse.roots[0]"},
		{name: "file scalar", yaml: "browse:\n  roots: x\n", wantErr: "browse.roots"},
		{name: "file null", yaml: "browse:\n  roots:\n", wantErr: "browse.roots"},
		{name: "file nested list", yaml: "browse:\n  roots: [[x]]\n", wantErr: "browse.roots"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := newFixture(t, tt.yaml, tt.vars).load()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Load error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadBrowseRootsWindowsCaseInsensitive(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "windows" {
		t.Skip("path case folding is Windows only")
	}

	f := newFixture(t, "", map[string]string{env("BROWSE_ROOTS"): `C:\Media;c:\media`})

	if _, err := f.load(); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("Load error = %v, want duplicate", err)
	}
}

func TestLoadInstallKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		yaml    string
		vars    map[string]string
		want    string
		wantErr string
	}{
		{name: "default", want: config.InstallKindStandard},
		{name: "file", yaml: "install:\n  kind: Docker\n", want: config.InstallKindDocker},
		{
			name: "env wins", yaml: "install:\n  kind: docker\n",
			vars: map[string]string{env("INSTALL_KIND"): "SYNOLOGY"}, want: config.InstallKindSynology,
		},
		{name: "bad env", vars: map[string]string{env("INSTALL_KIND"): "nas"}, wantErr: env("INSTALL_KIND")},
		{name: "bad file", yaml: "install:\n  kind: nas\n", wantErr: "install.kind"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := newFixture(t, tt.yaml, tt.vars).load()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load error = %v, want %q", err, tt.wantErr)
				}

				return
			}

			if err != nil || got.Install.Kind != tt.want {
				t.Errorf("Install.Kind = %q (err %v), want %q", got.Install.Kind, err, tt.want)
			}
		})
	}
}

func TestDefaultBrowseRoots(t *testing.T) {
	t.Parallel()

	present := func(existing ...string) func(string) bool {
		return func(path string) bool { return slices.Contains(existing, path) }
	}
	linux := func(home string) paths.Environment {
		return paths.Environment{GOOS: "linux", LookupEnv: func(key string) (string, bool) { return home, key == "HOME" }}
	}

	std := config.InstallKindStandard

	tests := []struct {
		name   string
		env    paths.Environment
		kind   string
		exists func(string) bool
		want   []string
	}{
		{
			name: "linux existing only", env: linux("/home/u"), kind: std, exists: present("/media", "/srv", "/home/u"),
			want: []string{"/media", "/srv", "/home/u"},
		},
		{name: "home cleaned", env: linux("/home/u/"), kind: std, exists: present("/home/u"), want: []string{"/home/u"}},
		{name: "home already listed", env: linux("/mnt/"), kind: std, exists: present("/mnt"), want: []string{"/mnt"}},
		{name: "relative home", env: linux("home/u"), kind: std, exists: present("home/u"), want: []string{}},
		{name: "filesystem root home", env: linux("/"), kind: std, exists: present("/", "/srv"), want: []string{"/srv"}},
		{
			name: "docker skips home", env: linux("/home/u"), kind: config.InstallKindDocker,
			exists: present("/media", "/home/u"), want: []string{"/media"},
		},
		{
			name: "synology skips home", env: linux("/home/u"), kind: config.InstallKindSynology,
			exists: present("/srv", "/home/u"), want: []string{"/srv"},
		},
		{name: "linux none", env: linux("/home/u"), exists: present(), want: []string{}},
		{name: "linux no home", env: paths.Environment{GOOS: "linux"}, exists: present("/mnt"), want: []string{"/mnt"}},
		{
			name: "windows drives", env: paths.Environment{GOOS: "windows"}, exists: present(`C:\`, `D:\`, `Z:\`),
			want: []string{`C:\`, `D:\`, `Z:\`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := config.DefaultBrowseRoots(tt.env, tt.kind, tt.exists); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DefaultBrowseRoots = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadBrowseRootsDefaultUsesInjectedProbe(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "", nil)
	f.env.GOOS = runtime.GOOS

	var probed []string

	got, err := config.Load(config.Options{
		ConfigDirFlag: f.configDir, Environment: f.env,
		PathExists: func(path string) bool { probed = append(probed, path); return false },
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.Browse.Roots == nil || len(got.Browse.Roots) != 0 || len(probed) == 0 {
		t.Errorf("Browse.Roots = %#v, probed %v; want empty non-nil and a probe", got.Browse.Roots, probed)
	}
}
