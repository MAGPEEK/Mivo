package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func sampleCase(name, output string) mediaCase {
	return mediaCase{
		Name: name, Category: categoryVideo, Output: output,
		Source: standardSource(),
		Expect: expect("mkv", probeMatroska, h264Video(), audioOf("aac")),
	}
}

func TestNewManifestSortsEntriesByName(t *testing.T) {
	t.Parallel()

	entries := []manifestEntry{
		newEntry(sampleCase("video-c", "video/c.mkv"), statusGenerated, ""),
		newEntry(sampleCase("video-a", "video/a.mkv"), statusFailed, "boom"),
		newSkippedEntry(sampleCase("video-b", "video/b.mkv"), skipMissingCapability, "missing"),
	}

	result := newManifest("9.0", entries)

	names := make([]string, 0, len(result.Cases))
	for _, entry := range result.Cases {
		names = append(names, entry.Name)
	}

	if !slices.Equal(names, []string{"video-a", "video-b", "video-c"}) {
		t.Errorf("order = %v", names)
	}

	if entries[0].Name != "video-c" {
		t.Error("newManifest reordered the caller's slice")
	}

	if result.count(statusGenerated) != 1 || result.count(statusSkipped) != 1 || result.count(statusFailed) != 1 {
		t.Errorf("counts wrong: %+v", result)
	}
}

func TestManifestJSONShape(t *testing.T) {
	t.Parallel()

	built := sampleCase("video-mkv-h264-aac", "video/h264_aac.mkv")
	built.Encoders = []string{"libx264", "aac"}

	result := newManifest("9.0.2", []manifestEntry{
		newBuiltEntry(built),
		newSkippedEntry(knownGap("video-rmvb-rv40", "video/rv40.rmvb",
			expect("rm", "rm", &videoExpectation{Codec: "rv40"}, nil), "no RV40 encoder"),
			skipKnownGap, gapReasonPrefix+"no RV40 encoder"),
	})

	got, err := result.encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	const want = `{
  "schemaVersion": 2,
  "ffmpegVersion": "9.0.2",
  "cases": [
    {
      "name": "video-mkv-h264-aac",
      "category": "video",
      "file": "video/h264_aac.mkv",
      "status": "generated",
      "reason": "",
      "encoders": [
        "libx264",
        "aac"
      ],
      "expected": {
        "container": "mkv",
        "probeFormat": "matroska,webm",
        "durationSeconds": 2,
        "video": {
          "codec": "h264",
          "profile": "High",
          "pixelFormat": "yuv420p",
          "width": 320,
          "height": 240
        },
        "audio": [
          {
            "codec": "aac",
            "sampleRate": 48000,
            "channels": 2
          }
        ]
      }
    },
    {
      "name": "video-rmvb-rv40",
      "category": "video",
      "file": "video/rv40.rmvb",
      "status": "skipped",
      "skipKind": "known-gap",
      "reason": "known gap (ADR 0021): no RV40 encoder",
      "expected": {
        "container": "rm",
        "probeFormat": "rm",
        "video": {
          "codec": "rv40"
        }
      }
    }
  ]
}
`
	if string(got) != want {
		t.Errorf("manifest JSON =\n%s\nwant\n%s", got, want)
	}
}

func TestManifestJSONShapeOfTracksChaptersAndSidecars(t *testing.T) {
	t.Parallel()

	entry := newBuiltEntry(findCase(t, "multitrack-mkv-eng-deu"))
	entry.Expected.Chapters = []chapterExpectation{{Title: "One", StartSeconds: 0, EndSeconds: 1.5}}
	entry.Expected.Tags = map[string]string{"title": "T"}
	entry.Expected.Sidecars = []sidecarExpectation{{File: "multitrack/a.srt", ProbeFormat: "srt", Codec: "subrip"}}

	data, err := json.Marshal(entry.Expected)
	if err != nil {
		t.Fatal(err)
	}

	const want = `{"container":"mkv","probeFormat":"matroska,webm","durationSeconds":2,` +
		`"video":{"codec":"h264","profile":"High","pixelFormat":"yuv420p","width":320,"height":240,` +
		`"track":{"default":true,"forced":false}},` +
		`"audio":[{"codec":"aac","sampleRate":48000,"channels":2,"track":{"language":"eng","default":true,"forced":false}},` +
		`{"codec":"aac","sampleRate":48000,"channels":2,"track":{"language":"deu","default":false,"forced":false}}],` +
		`"subtitles":[{"codec":"subrip","track":{"language":"eng","default":false,"forced":false}},` +
		`{"codec":"subrip","track":{"language":"deu","default":false,"forced":true}}],` +
		`"chapters":[{"title":"One","startSeconds":0,"endSeconds":1.5}],"tags":{"title":"T"},` +
		`"sidecars":[{"file":"multitrack/a.srt","probeFormat":"srt","codec":"subrip"}]}`

	if string(data) != want {
		t.Errorf("expected JSON =\n%s\nwant\n%s", data, want)
	}

	// Consumers decode the manifest; the nested track objects must survive a round trip.
	var decoded expectation
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if decoded.Subtitles[1].Track == nil || !decoded.Subtitles[1].Track.Forced {
		t.Errorf("decoded subtitles = %+v", decoded.Subtitles)
	}
}

func TestWriteManifestReplacesTheFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir() + "/nested"

	for _, version := range []string{"first", "second"} {
		if err := writeManifest(dir, newManifest(version, nil)); err != nil {
			t.Fatalf("writeManifest: %v", err)
		}
	}

	data, err := fs.ReadFile(os.DirFS(dir), manifestFileName)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	var decoded manifest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}

	if decoded.FFmpegVersion != "second" || decoded.SchemaVersion != manifestSchemaVersion {
		t.Errorf("manifest = %+v", decoded)
	}

	if decoded.Cases == nil {
		t.Error(`an empty manifest must encode "cases": [] instead of null`)
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}

	if len(files) != 1 {
		t.Errorf("temporary manifest files left behind: %v", files)
	}
}

func TestWriteManifestIsGroupReadable(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Windows has no group permission bits; Chmod only toggles read-only")
	}

	dir := t.TempDir()
	if err := writeManifest(dir, newManifest("x", nil)); err != nil {
		t.Fatalf("writeManifest: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, manifestFileName))
	if err != nil {
		t.Fatal(err)
	}

	if got := info.Mode().Perm(); got != outputFilePermissions {
		t.Errorf("manifest permissions = %v, want %v", got, os.FileMode(outputFilePermissions))
	}
}

func TestWriteManifestFailsWhenDirectoryIsAFile(t *testing.T) {
	t.Parallel()

	blocker := t.TempDir() + "/file"
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := writeManifest(blocker, newManifest("x", nil)); err == nil {
		t.Error("writeManifest succeeded inside a regular file")
	}
}

func TestWriteFileAtomicallyFailsWithoutDirectory(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "missing", "file.txt")
	if err := writeFileAtomically(target, []byte("x")); err == nil {
		t.Error("writeFileAtomically succeeded in a missing directory")
	}
}

func TestReadPreviousEncoders(t *testing.T) {
	t.Parallel()

	built := sampleCase("video-built", "video/built.mkv")
	built.Encoders = []string{"libaom-av1", "libopus"}
	skipped := sampleCase("video-skipped", "video/skipped.mkv")
	skipped.Encoders = []string{"libx264"}

	dir := t.TempDir()
	if err := writeManifest(dir, newManifest("9.0.2", []manifestEntry{
		newBuiltEntry(built),
		newKeptEntry(sampleCase("video-kept", "video/kept.mkv"), nil),
		newSkippedEntry(skipped, skipMissingCapability, "missing"),
		newEntry(sampleCase("video-failed", "video/failed.mkv"), statusFailed, "boom"),
	})); err != nil {
		t.Fatal(err)
	}

	previous, err := readPreviousEncoders(dir)
	if err != nil {
		t.Fatalf("readPreviousEncoders: %v", err)
	}

	tests := []struct {
		c    mediaCase
		want []string
	}{
		{c: built, want: []string{"libaom-av1", "libopus"}},
		// Another file under the same name was not built by the earlier run.
		{c: sampleCase("video-built", "video/renamed.mkv")},
		{c: sampleCase("video-kept", "video/kept.mkv")},
		{c: skipped},
		{c: sampleCase("video-failed", "video/failed.mkv")},
		{c: sampleCase("video-new", "video/new.mkv")},
	}

	for _, test := range tests {
		if got := previous.of(test.c); !slices.Equal(got, test.want) {
			t.Errorf("encoders of %s (%s) = %v, want %v", test.c.Name, test.c.Output, got, test.want)
		}
	}
}

func TestReadPreviousEncodersWithoutAUsableManifest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		prepare func(t *testing.T, dir string)
		wantErr string
	}{
		{name: "missing", prepare: func(*testing.T, string) {}},
		{
			name:    "malformed",
			prepare: func(t *testing.T, dir string) { writeFile(t, filepath.Join(dir, manifestFileName), "[") },
			wantErr: "decode",
		},
		{
			name: "other schema",
			prepare: func(t *testing.T, dir string) {
				writeFile(t, filepath.Join(dir, manifestFileName), `{"schemaVersion": 1}`)
			},
			wantErr: "schema version 1, want 2",
		},
		{
			name: "unreadable",
			prepare: func(t *testing.T, dir string) {
				if err := os.Mkdir(filepath.Join(dir, manifestFileName), 0o750); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "read",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			test.prepare(t, dir)

			previous, err := readPreviousEncoders(dir)
			if len(previous) != 0 {
				t.Errorf("previous = %v, want none", previous)
			}

			if test.wantErr == "" && err != nil || test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)) {
				t.Errorf("error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestKeptEntriesAreGeneratedInTheManifest(t *testing.T) {
	t.Parallel()

	kept := newKeptEntry(sampleCase("video-a", "video/a.mkv"), []string{"libx264"})
	built := newBuiltEntry(sampleCase("video-a", "video/a.mkv"))
	built.Encoders = []string{"libx264"}

	keptJSON, err := json.Marshal(kept)
	if err != nil {
		t.Fatal(err)
	}

	builtJSON, err := json.Marshal(built)
	if err != nil {
		t.Fatal(err)
	}

	// "kept" only labels progress and summary; consumers see a generated file either way.
	if string(keptJSON) != string(builtJSON) || kept.progressLabel() != "kept" || built.progressLabel() != "generated" {
		t.Errorf("kept = %s (%s), built = %s (%s)", keptJSON, kept.progressLabel(), builtJSON, built.progressLabel())
	}
}
