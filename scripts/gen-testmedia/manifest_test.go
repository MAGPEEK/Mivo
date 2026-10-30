package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"slices"
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
		newEntry(sampleCase("video-b", "video/b.mkv"), statusSkipped, "missing"),
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

	result := newManifest("9.0.2", []manifestEntry{
		newEntry(sampleCase("video-mkv-h264-aac", "video/h264_aac.mkv"), statusGenerated, ""),
		newEntry(mediaCase{
			Name: "video-rm-rv20", Category: categoryVideo, Output: "video/rv20.rm",
			Source: standardSource().withAudio(realAudio144SampleRate, monoChannels),
			Expect: expect("rm", "rm", videoOf("rv20", ""), audioOf("ra_144")),
		}, statusSkipped, "ffmpeg lacks encoder rv20"),
	})

	got, err := result.encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	const want = `{
  "schemaVersion": 1,
  "ffmpegVersion": "9.0.2",
  "cases": [
    {
      "name": "video-mkv-h264-aac",
      "category": "video",
      "file": "video/h264_aac.mkv",
      "status": "generated",
      "reason": "",
      "expected": {
        "container": "mkv",
        "probeFormat": "matroska,webm",
        "video": {
          "codec": "h264",
          "profile": "High",
          "pixelFormat": "yuv420p",
          "width": 320,
          "height": 240
        },
        "audio": {
          "codec": "aac",
          "sampleRate": 48000,
          "channels": 2
        }
      }
    },
    {
      "name": "video-rm-rv20",
      "category": "video",
      "file": "video/rv20.rm",
      "status": "skipped",
      "reason": "ffmpeg lacks encoder rv20",
      "expected": {
        "container": "rm",
        "probeFormat": "rm",
        "video": {
          "codec": "rv20",
          "width": 320,
          "height": 240
        },
        "audio": {
          "codec": "ra_144",
          "sampleRate": 8000,
          "channels": 1
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
