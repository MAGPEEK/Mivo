package main

import (
	"debug/elf"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/magpeek/mivo/packaging/synology/internal/spkmeta"
)

// refusalCase breaks one input of a valid x86_64 staging run.
type refusalCase struct {
	name string
	// breakInput changes the options or the files they name.
	breakInput func(t *testing.T, opts *options, pkg *spkmeta.Package)
	// want is a substring of the error, normally the name of the input.
	want    string
	wantErr error
}

func refusalCases() []refusalCase {
	return []refusalCase{
		{
			name:       "unknown family",
			breakInput: func(_ *testing.T, opts *options, _ *spkmeta.Package) { opts.family = "mips" },
			want:       `-family "mips"`, wantErr: spkmeta.ErrUnknownFamily,
		},
		{
			name:       "invalid version",
			breakInput: func(_ *testing.T, opts *options, _ *spkmeta.Package) { opts.version = "v1.0" },
			want:       `-version "v1.0"`, wantErr: spkmeta.ErrInvalidVersion,
		},
		{
			name:       "missing binary",
			breakInput: func(_ *testing.T, opts *options, _ *spkmeta.Package) { opts.binary += ".missing" },
			want:       "-binary", wantErr: os.ErrNotExist,
		},
		{
			name: "binary for the wrong machine",
			breakInput: func(t *testing.T, opts *options, _ *spkmeta.Package) {
				writeFile(t, opts.binary, fakeELF(t, elf.ELFCLASS64, elf.EM_AARCH64), testExecutableMode)
			},
			want: "-binary", wantErr: errWrongMachine,
		},
		{
			name: "binary is not ELF",
			breakInput: func(t *testing.T, opts *options, _ *spkmeta.Package) {
				writeFile(t, opts.binary, []byte("#!/bin/sh\n"), testExecutableMode)
			},
			want: "-binary", wantErr: errWrongMachine,
		},
		{
			name:       "relocatable object as binary",
			breakInput: writeBinarySpec(elfSpec{class: elf.ELFCLASS64, machine: elf.EM_X86_64, fileType: elf.ET_REL}),
			want:       "-binary", wantErr: errNotExecutableType,
		},
		{
			name: "dynamically linked binary",
			breakInput: writeBinarySpec(elfSpec{
				class: elf.ELFCLASS64, machine: elf.EM_X86_64, fileType: elf.ET_EXEC,
				programs: []elf.ProgType{elf.PT_PHDR, elf.PT_INTERP, elf.PT_LOAD},
			}),
			want: "-binary", wantErr: errDynamicallyLinked,
		},
		{
			name: "dynamically linked PIE binary",
			breakInput: writeBinarySpec(elfSpec{
				class: elf.ELFCLASS64, machine: elf.EM_X86_64, fileType: elf.ET_DYN,
				programs: []elf.ProgType{elf.PT_INTERP, elf.PT_LOAD},
			}),
			want: "-binary", wantErr: errDynamicallyLinked,
		},
		{
			name: "dynamically linked ffmpeg of a selected build",
			breakInput: func(t *testing.T, opts *options, _ *spkmeta.Package) {
				writeFile(t, filepath.Join(opts.ffmpegDir, ffmpegName), fakeELFFile(t, elfSpec{
					class: elf.ELFCLASS64, machine: elf.EM_X86_64, fileType: elf.ET_EXEC,
					programs: []elf.ProgType{elf.PT_INTERP},
				}), testExecutableMode)
			},
			want: ffmpegName, wantErr: errDynamicallyLinked,
		},
		{
			name: "ffprobe for the wrong class",
			breakInput: func(t *testing.T, opts *options, _ *spkmeta.Package) {
				writeFile(t, filepath.Join(opts.ffmpegDir, ffprobeName), fakeELF(t, elf.ELFCLASS32, elf.EM_X86_64), testExecutableMode)
			},
			want: "ffprobe", wantErr: errWrongMachine,
		},
		{
			name:       "missing FFmpeg directory",
			breakInput: func(_ *testing.T, opts *options, _ *spkmeta.Package) { opts.ffmpegDir += ".missing" },
			want:       "-ffmpeg-dir", wantErr: os.ErrNotExist,
		},
		{
			name:       "FFmpeg directory is a file",
			breakInput: func(_ *testing.T, opts *options, _ *spkmeta.Package) { opts.ffmpegDir = opts.binary },
			want:       "is not a directory", wantErr: errUnexpectedInput,
		},
		{
			name:       "missing ffmpeg",
			breakInput: removeFFmpegFile(ffmpegName),
			want:       "-ffmpeg-dir", wantErr: os.ErrNotExist,
		},
		{
			name:       "missing SOURCE.txt",
			breakInput: removeFFmpegFile(sourceNoticeName),
			want:       sourceNoticeName, wantErr: os.ErrNotExist,
		},
		{
			name: "no licence text besides SOURCE.txt",
			breakInput: func(t *testing.T, opts *options, pkg *spkmeta.Package) {
				removeFFmpegFile(fakeLicenseGPL)(t, opts, pkg)
				removeFFmpegFile(fakeLicenseMD)(t, opts, pkg)
			},
			want: "holds no licence file", wantErr: errUnexpectedInput,
		},
		{
			name: "subdirectory in the FFmpeg directory",
			breakInput: func(t *testing.T, opts *options, _ *spkmeta.Package) {
				writeFile(t, filepath.Join(opts.ffmpegDir, "extra", "file"), []byte("x"), testFileMode)
			},
			want: "extra is not a licence file", wantErr: errUnexpectedInput,
		},
		{
			name: "licence file with an unsafe name",
			breakInput: func(t *testing.T, opts *options, _ *spkmeta.Package) {
				writeFile(t, filepath.Join(opts.ffmpegDir, "my licence"), []byte("x"), testFileMode)
			},
			want: "my licence is not a licence file", wantErr: errUnexpectedInput,
		},
		{
			name: "INFO-unsafe package value",
			breakInput: func(_ *testing.T, _ *options, pkg *spkmeta.Package) {
				pkg.Description = "Costs $5"
			},
			want: "DESCRIPTION", wantErr: spkmeta.ErrUnsafeValue,
		},
		{
			name: "value that leaves a build token behind",
			breakInput: func(_ *testing.T, _ *options, pkg *spkmeta.Package) {
				pkg.DisplayName = "Server @PORT@"
			},
			want: "@PORT@", wantErr: errLeftoverToken,
		},
		{
			name:       "missing repository",
			breakInput: func(t *testing.T, opts *options, _ *spkmeta.Package) { opts.repo = filepath.Join(t.TempDir(), "none") },
			want:       "-repo", wantErr: os.ErrNotExist,
		},
	}
}

func writeBinarySpec(spec elfSpec) func(*testing.T, *options, *spkmeta.Package) {
	return func(t *testing.T, opts *options, _ *spkmeta.Package) {
		t.Helper()

		writeFile(t, opts.binary, fakeELFFile(t, spec), testExecutableMode)
	}
}

func removeFFmpegFile(name string) func(*testing.T, *options, *spkmeta.Package) {
	return func(t *testing.T, opts *options, _ *spkmeta.Package) {
		t.Helper()

		if err := os.Remove(filepath.Join(opts.ffmpegDir, name)); err != nil {
			t.Fatalf("remove %s: %v", name, err)
		}
	}
}

func TestStageRefusesBrokenInputs(t *testing.T) {
	t.Parallel()

	for _, test := range refusalCases() {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			opts := testOptions(t, "x86_64", writeInputs(t, "x86_64", fakeSourceSelected))
			pkg := spkmeta.DefaultPackage()
			test.breakInput(t, &opts, &pkg)

			assertRefused(t, opts, pkg, expectation{test.want, test.wantErr})
		})
	}
}

// expectation is the refusal a test expects: an error substring and a wrapped error.
type expectation struct {
	want    string
	wantErr error
}

func assertRefused(t *testing.T, opts options, pkg spkmeta.Package, expected expectation) {
	t.Helper()

	want, wantErr := expected.want, expected.wantErr

	_, err := stage(opts, pkg, io.Discard)
	if err == nil {
		t.Fatal("stage succeeded, want a refusal")
	}

	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), want) {
		t.Errorf("error = %v, want %v naming %q", err, wantErr, want)
	}

	if _, statErr := os.Stat(opts.out); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("refused run created %s", opts.out)
	}
}

func TestStageRefusesNonExecutableInputs(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Windows file systems carry no executable bit")
	}

	opts := testOptions(t, "x86_64", writeInputs(t, "x86_64", fakeSourceSelected))
	if err := os.Chmod(opts.binary, testFileMode); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	assertRefused(t, opts, spkmeta.DefaultPackage(), expectation{"-binary", errNotExecutable})
}

// templateCase breaks one template of a copied repository.
type templateCase struct {
	name     string
	template string
	content  string
	want     string
	wantErr  error
}

func TestStageRefusesBrokenTemplates(t *testing.T) {
	t.Parallel()

	tests := []templateCase{
		{name: "CRLF in a script", template: "scripts/postinst", content: "#!/bin/sh\r\nexit 0\r\n", want: "scripts/postinst", wantErr: errCarriageReturn},
		{name: "CRLF in conf", template: "conf/privilege", content: "{}\r\n", want: "conf/privilege", wantErr: errCarriageReturn},
		{name: "CRLF in a wizard script", template: "WIZARD_UIFILES/install_uifile.sh", content: "#!/bin/sh\r\n", want: "install_uifile.sh", wantErr: errCarriageReturn},
		{name: "non-ASCII in SynoBuildConf", template: "SynoBuildConf/depends", content: "# § 1\n", want: "SynoBuildConf/depends", wantErr: spkmeta.ErrNonASCII},
		{name: "non-ASCII in INFO.sh", template: "INFO.sh", content: "#!/bin/bash\n# grüße\n", want: "INFO.sh", wantErr: errNonASCII},
		{name: "unknown token", template: "conf/resource", content: `{"x": "@UNKNOWN@"}`, want: "conf/resource", wantErr: spkmeta.ErrUnknownToken},
		{name: "invalid JSON", template: "WIZARD_UIFILES/install_uifile", content: "[{]\n", want: "install_uifile", wantErr: errInvalidJSON},
		{name: "template without a rule", template: "extra/notes.txt", content: "x\n", want: "extra/notes.txt", wantErr: errUnexpectedInput},
		{name: "package.env template next to the generated one", template: "scripts/package.env", content: "ID='x'\n", want: "two sources for scripts/package.env", wantErr: errUnexpectedInput},
		{name: "paths that differ only in case", template: "scripts/Package.env", content: "ID='x'\n", want: "two sources for scripts/package.env", wantErr: errUnexpectedInput},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			opts := testOptions(t, "x86_64", writeInputs(t, "x86_64", fakeSourceSelected))
			opts.repo = copyRepository(t)
			writeFile(t, filepath.Join(opts.repo, filepath.FromSlash(packageTemplatesDir), filepath.FromSlash(test.template)),
				[]byte(test.content), testFileMode)

			assertRefused(t, opts, spkmeta.DefaultPackage(), expectation{test.want, test.wantErr})
		})
	}
}

func TestStageRefusesMissingLicense(t *testing.T) {
	t.Parallel()

	opts := testOptions(t, "x86_64", writeInputs(t, "x86_64", fakeSourceSelected))
	opts.repo = copyRepository(t)

	if err := os.Remove(filepath.Join(opts.repo, licensePath)); err != nil {
		t.Fatalf("remove LICENSE: %v", err)
	}

	assertRefused(t, opts, spkmeta.DefaultPackage(), expectation{"-repo", os.ErrNotExist})
}

// TestForceNeverReplacesForeignDirectories covers the directories -force must keep: one that
// holds an input of the run and one that is not a staged project.
func TestForceNeverReplacesForeignDirectories(t *testing.T) {
	t.Parallel()

	inputs := writeInputs(t, "x86_64", fakeSourceSelected)

	for _, out := range []string{filepath.Dir(inputs.binary), t.TempDir()} {
		writeFile(t, filepath.Join(out, "keep"), []byte("x"), testFileMode)

		opts := testOptions(t, "x86_64", inputs)
		opts.out, opts.force = out, true

		if _, err := stage(opts, spkmeta.DefaultPackage(), io.Discard); !errors.Is(err, errUnsafeOutput) {
			t.Errorf("-out %s: error = %v, want %v", out, err, errUnsafeOutput)
		}

		if _, err := os.Stat(filepath.Join(out, "keep")); err != nil {
			t.Errorf("-out %s damaged: %v", out, err)
		}
	}
}

func TestStageUsesAnExistingEmptyProject(t *testing.T) {
	t.Parallel()

	opts := testOptions(t, "x86_64", writeInputs(t, "x86_64", fakeSourceSelected))
	if err := os.Mkdir(opts.out, testDirectoryMode); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if _, err := stage(opts, spkmeta.DefaultPackage(), io.Discard); err != nil {
		t.Fatalf("stage: %v", err)
	}
}

func TestStageRefusesAProjectFileInTheWay(t *testing.T) {
	t.Parallel()

	opts := testOptions(t, "x86_64", writeInputs(t, "x86_64", fakeSourceSelected))
	writeFile(t, opts.out, []byte("x"), testFileMode)

	if _, err := stage(opts, spkmeta.DefaultPackage(), io.Discard); err == nil || !strings.Contains(err.Error(), "-out") {
		t.Fatalf("error = %v, want one naming -out", err)
	}
}

func TestForceNeverReplacesTheRepository(t *testing.T) {
	t.Parallel()

	repo := copyRepository(t)

	for _, out := range []string{repo, filepath.Dir(repo)} {
		opts := testOptions(t, "x86_64", writeInputs(t, "x86_64", fakeSourceSelected))
		opts.repo, opts.out, opts.force = repo, out, true

		_, err := stage(opts, spkmeta.DefaultPackage(), io.Discard)
		if !errors.Is(err, errUnsafeOutput) {
			t.Errorf("-out %s: error = %v, want %v", out, err, errUnsafeOutput)
		}
	}

	if _, err := os.Stat(filepath.Join(repo, licensePath)); err != nil {
		t.Errorf("repository damaged: %v", err)
	}
}

func TestFamilyELFRejectsUnknownFamily(t *testing.T) {
	t.Parallel()

	if _, err := familyELF("mips"); !errors.Is(err, errUnexpectedInput) {
		t.Errorf("familyELF(mips) error = %v, want %v", err, errUnexpectedInput)
	}
}
