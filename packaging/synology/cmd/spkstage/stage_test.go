package main

import (
	"debug/elf"
	"encoding/json"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/platform/brand"
	"github.com/magpeek/mivo/packaging/synology/internal/spkicon"
	"github.com/magpeek/mivo/packaging/synology/internal/spkmeta"
)

// goldenTree is the staged project of every family (toolkit README "Staged project"). The
// families differ only in content (build.env ARCH, the binaries), not in the file list.
func goldenTree() []treeEntry {
	const (
		x = testExecutableMode
		f = testFileMode
		d = testDirectoryMode
	)

	tree := []treeEntry{
		{".spkstage", f},
		{"INFO.sh", x},
		{"LICENSE", f},
		{"PACKAGE_ICON.PNG", f},
		{"PACKAGE_ICON_256.PNG", f},
		{"SynoBuildConf/", d},
		{"SynoBuildConf/build", x},
		{"SynoBuildConf/depends", f},
		{"SynoBuildConf/install", x},
		{"WIZARD_UIFILES/", d},
		{"WIZARD_UIFILES/install_uifile", f},
		{"WIZARD_UIFILES/install_uifile_ger", f},
		{"WIZARD_UIFILES/uninstall_uifile", f},
		{"WIZARD_UIFILES/uninstall_uifile_ger", f},
		{"build.env", f},
		{"conf/", d},
		{"conf/privilege", f},
		{"conf/resource", f},
		{"prebuilt/", d},
		{"prebuilt/bin/", d},
		{"prebuilt/bin/" + brand.ID, x},
		{"prebuilt/bin/ffmpeg", x},
		{"prebuilt/bin/ffprobe", x},
		{"prebuilt/port_conf/", d},
		{"prebuilt/port_conf/" + brand.ID + ".sc", f},
		{"prebuilt/share/", d},
		{"prebuilt/share/licenses/", d},
		{"prebuilt/share/licenses/ffmpeg/", d},
		{"prebuilt/share/licenses/ffmpeg/COPYING.GPLv3", f},
		{"prebuilt/share/licenses/ffmpeg/LICENSE.md", f},
		{"prebuilt/share/licenses/ffmpeg/SOURCE.txt", f},
		{"scripts/", d},
		{"scripts/common.sh", x},
		{"scripts/package.env", f},
		{"scripts/postinst", x},
		{"scripts/postuninst", x},
		{"scripts/postupgrade", x},
		{"scripts/preinst", x},
		{"scripts/preuninst", x},
		{"scripts/preupgrade", x},
		{"scripts/start-stop-status", x},
	}
	sortTree(tree)

	return tree
}

func familyNames() []string {
	names := make([]string, 0, len(spkmeta.Families()))
	for _, family := range spkmeta.Families() {
		names = append(names, family.Name)
	}

	return names
}

func TestStagedTreeMatchesGoldenList(t *testing.T) {
	t.Parallel()

	for _, family := range familyNames() {
		t.Run(family, func(t *testing.T) {
			t.Parallel()

			opts := testOptions(t, family, writeInputs(t, family, fakeSourceSelected))
			assertTree(t, "planned", plannedTree(planFor(t, opts)), goldenTree())

			if _, err := stage(opts, spkmeta.DefaultPackage(), io.Discard); err != nil {
				t.Fatalf("stage: %v", err)
			}

			assertTree(t, "on disk", diskTree(t, opts.out), goldenTreeOnDisk())
		})
	}
}

func planFor(t *testing.T, opts options) []entry {
	t.Helper()

	build, err := spkmeta.NewBuild(spkmeta.DefaultPackage(), spkmeta.BuildOptions{ServerVersion: opts.version, Family: opts.family})
	if err != nil {
		t.Fatalf("NewBuild: %v", err)
	}

	entries, err := planProject(opts, build, io.Discard)
	if err != nil {
		t.Fatalf("planProject: %v", err)
	}

	return entries
}

// goldenTreeOnDisk is the golden list as diskTree reports it: without modes on Windows.
func goldenTreeOnDisk() []treeEntry {
	want := goldenTree()
	if runtime.GOOS == "windows" {
		for index := range want {
			want[index].mode = 0
		}
	}

	return want
}

func assertTree(t *testing.T, label string, got, want []treeEntry) {
	t.Helper()

	if slices.Equal(got, want) {
		return
	}

	t.Errorf("%s tree differs from the golden list", label)

	for index := range max(len(got), len(want)) {
		var gotLine, wantLine string
		if index < len(got) {
			gotLine = got[index].mode.String() + " " + got[index].path
		}

		if index < len(want) {
			wantLine = want[index].mode.String() + " " + want[index].path
		}

		if gotLine != wantLine {
			t.Errorf("  entry %d: got %q, want %q", index, gotLine, wantLine)
		}
	}
}

func TestStagedBuildEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		family, version, wantArch, wantDSM, wantBeta string
		revision                                     int
	}{
		{family: "x86_64", version: "0.1.0-alpha.2", wantArch: "x86_64", wantDSM: "0.1.0-1020", wantBeta: "yes"},
		{family: "armv8", version: "1.2.3", revision: 1, wantArch: "armv8", wantDSM: "1.2.3-9001", wantBeta: "no"},
		{family: "armv7", version: "0.1.0-rc.1", wantArch: "armv7 armada38x monaco", wantDSM: "0.1.0-3010", wantBeta: "yes"},
	}

	for _, test := range tests {
		t.Run(test.family, func(t *testing.T) {
			t.Parallel()

			opts := testOptions(t, test.family, writeInputs(t, test.family, fakeSourceSelected))
			opts.version = test.version
			opts.packageRevision = test.revision

			if _, err := stage(opts, spkmeta.DefaultPackage(), io.Discard); err != nil {
				t.Fatalf("stage: %v", err)
			}

			buildEnv := readStaged(t, opts.out, buildEnvPath)
			for _, line := range []string{
				"ARCH='" + test.wantArch + "'",
				"DSM_VERSION='" + test.wantDSM + "'",
				"BETA='" + test.wantBeta + "'",
				"VERSION='" + test.version + "'",
				"ID='" + brand.ID + "'",
			} {
				if !slices.Contains(strings.Split(buildEnv, "\n"), line) {
					t.Errorf("build.env lacks %s:\n%s", line, buildEnv)
				}
			}
		})
	}
}

func TestStagedFilesAreRenderedAndValid(t *testing.T) {
	t.Parallel()

	opts := testOptions(t, "armv7", writeInputs(t, "armv7", fakeSourceSelected))
	if _, err := stage(opts, spkmeta.DefaultPackage(), io.Discard); err != nil {
		t.Fatalf("stage: %v", err)
	}

	tokens := leftoverToken

	for _, item := range diskTree(t, opts.out) {
		if strings.HasSuffix(item.path, "/") || strings.HasPrefix(item.path, "prebuilt/bin/") {
			continue
		}

		content := readStaged(t, opts.out, item.path)
		if token := tokens.FindString(content); token != "" {
			t.Errorf("%s contains the token %s", item.path, token)
		}

		if isJSONPath(item.path) && !json.Valid([]byte(content)) {
			t.Errorf("%s is not valid JSON", item.path)
		}
	}

	sc := readStaged(t, opts.out, "prebuilt/port_conf/"+brand.ID+".sc")
	if !strings.HasPrefix(sc, "["+brand.ID+"]\n") || !strings.Contains(sc, `dst.ports="8460/tcp"`) {
		t.Errorf("service file not rendered:\n%s", sc)
	}

	assertIcons(t, opts.out)
}

func isJSONPath(name string) bool {
	return strings.HasPrefix(name, "conf/") || (strings.HasPrefix(name, "WIZARD_UIFILES/") && !strings.HasSuffix(name, ".sh"))
}

func assertIcons(t *testing.T, root string) {
	t.Helper()

	for _, icon := range spkicon.PackageIcons() {
		file, err := os.Open(filepath.Join(root, icon.FileName)) //nolint:gosec // staged icon of the test
		if err != nil {
			t.Fatalf("open %s: %v", icon.FileName, err)
		}

		config, err := png.DecodeConfig(file)
		_ = file.Close()

		if err != nil {
			t.Fatalf("decode %s: %v", icon.FileName, err)
		}

		if config.Width != icon.Size || config.Height != icon.Size {
			t.Errorf("%s is %dx%d, want %dx%d", icon.FileName, config.Width, config.Height, icon.Size, icon.Size)
		}
	}
}

func TestStagingIsReproducible(t *testing.T) {
	t.Parallel()

	inputs := writeInputs(t, "x86_64", fakeSourceSelected)
	first := testOptions(t, "x86_64", inputs)
	second := testOptions(t, "x86_64", inputs)

	for _, opts := range []options{first, second} {
		if _, err := stage(opts, spkmeta.DefaultPackage(), io.Discard); err != nil {
			t.Fatalf("stage: %v", err)
		}
	}

	firstTree, secondTree := snapshot(t, first.out), snapshot(t, second.out)
	if len(firstTree) == 0 {
		t.Fatal("empty staged tree")
	}

	for name, description := range firstTree {
		if secondTree[name] != description {
			t.Errorf("%s differs between two runs", name)
		}
	}

	if len(firstTree) != len(secondTree) {
		t.Errorf("runs staged %d and %d entries", len(firstTree), len(secondTree))
	}
}

func TestEveryEntryGetsTheSourceDateEpoch(t *testing.T) {
	t.Parallel()

	opts := testOptions(t, "armv8", writeInputs(t, "armv8", fakeSourceSelected))
	if _, err := stage(opts, spkmeta.DefaultPackage(), io.Discard); err != nil {
		t.Fatalf("stage: %v", err)
	}

	want := time.Unix(testEpoch, 0)

	for _, item := range append(diskTree(t, opts.out), treeEntry{path: "."}) {
		info, err := os.Stat(filepath.Join(opts.out, filepath.FromSlash(item.path)))
		if err != nil {
			t.Fatalf("stat %s: %v", item.path, err)
		}

		if !info.ModTime().Equal(want) {
			t.Errorf("%s has time %s, want %s", item.path, info.ModTime().UTC(), want.UTC())
		}
	}
}

func TestInterimFFmpegIsStagedWithAWarning(t *testing.T) {
	t.Parallel()

	opts := testOptions(t, "x86_64", writeInputs(t, "x86_64", fakeSourceInterim))

	var stderr strings.Builder
	if _, err := stage(opts, spkmeta.DefaultPackage(), &stderr); err != nil {
		t.Fatalf("stage: %v", err)
	}

	if !strings.Contains(stderr.String(), "warning:") || !strings.Contains(stderr.String(), "interim FFmpeg build") {
		t.Errorf("stderr = %q, want an interim warning", stderr.String())
	}

	if got := readStaged(t, opts.out, ffmpegLicenseDir+"/"+sourceNoticeName); got != fakeSourceInterim {
		t.Errorf("SOURCE.txt = %q, want it unchanged", got)
	}
}

// TestStaticPIEIsAccepted stages a static-pie server binary: ET_DYN without PT_INTERP.
func TestStaticPIEIsAccepted(t *testing.T) {
	t.Parallel()

	opts := testOptions(t, "x86_64", writeInputs(t, "x86_64", fakeSourceSelected))
	writeFile(t, opts.binary, fakeELFFile(t, elfSpec{
		class: elf.ELFCLASS64, machine: elf.EM_X86_64, fileType: elf.ET_DYN,
		programs: []elf.ProgType{elf.PT_PHDR, elf.PT_LOAD, elf.PT_DYNAMIC},
	}), testExecutableMode)

	if _, err := stage(opts, spkmeta.DefaultPackage(), io.Discard); err != nil {
		t.Fatalf("stage: %v", err)
	}
}

// TestDynamicInterimFFmpegIsStagedWithAWarning keeps the interim pins of ffmpeg/sources.lock
// usable until SPK-16: they link glibc dynamically, which a selected build may not.
func TestDynamicInterimFFmpegIsStagedWithAWarning(t *testing.T) {
	t.Parallel()

	opts := testOptions(t, "x86_64", writeInputs(t, "x86_64", fakeSourceInterim))
	dynamic := fakeELFFile(t, elfSpec{
		class: elf.ELFCLASS64, machine: elf.EM_X86_64, fileType: elf.ET_DYN,
		programs: []elf.ProgType{elf.PT_INTERP, elf.PT_LOAD},
	})
	writeFile(t, filepath.Join(opts.ffmpegDir, ffmpegName), dynamic, testExecutableMode)

	var stderr strings.Builder
	if _, err := stage(opts, spkmeta.DefaultPackage(), &stderr); err != nil {
		t.Fatalf("stage: %v", err)
	}

	if !strings.Contains(stderr.String(), ffmpegName+" is dynamically linked") {
		t.Errorf("stderr = %q, want a warning about the dynamic ffmpeg", stderr.String())
	}
}

func TestSelectedFFmpegGivesNoWarning(t *testing.T) {
	t.Parallel()

	opts := testOptions(t, "x86_64", writeInputs(t, "x86_64", fakeSourceSelected))

	var stderr strings.Builder
	if _, err := stage(opts, spkmeta.DefaultPackage(), &stderr); err != nil {
		t.Fatalf("stage: %v", err)
	}

	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}
