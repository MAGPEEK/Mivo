package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/corpustest"
	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

// discFolderName is the name the API reports for the clear DVD (the folder holding VIDEO_TS).
const discFolderName = "Movie (2019)"

// clearDVDRow is the disc row of the library manifest the smoke plays, as gen-testmedia writes it.
const clearDVDRow = `{"title":"Movie","path":"Movie (2019)/VIDEO_TS","layout":"video_ts",` +
	`"mainTitle":{"clips":["VTS_01_1.VOB"],"durationSeconds":6},` +
	`"streams":{"video":"mpeg2video","width":720,"height":576,"audio":["ac3"],"subtitles":["dvd_subtitle"]}}`

func writeDiscManifest(t *testing.T, rows ...string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "library-manifest.json")
	content := `{"schemaVersion":1,"discs":{"path":"discs","items":[` + strings.Join(rows, ",") + `]}}`

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write the library manifest: %v", err)
	}

	return path
}

func TestDiscCasesPlayTheClearDVDOnlyWhenAsked(t *testing.T) {
	t.Parallel()

	none, err := discCases("", corpustest.ChromiumDesktop)
	if err != nil || len(none) != 0 {
		t.Errorf("discCases without a manifest = %v, %v; want no case", none, err)
	}

	cases, err := discCases(writeDiscManifest(t, clearDVDRow), corpustest.FirefoxDesktop)
	if err != nil || len(cases) != 1 {
		t.Fatalf("discCases = %v, %v; want one case", cases, err)
	}

	disc := cases[0]
	want, _ := corpustest.ExpectedDisc(corpustest.DiscVideoTS, corpustest.FirefoxDesktop)

	if disc.clip.fileName() != discFolderName || disc.clip.duration() != 6*time.Second ||
		disc.clip.defaultAudioCodec() != "ac3" || disc.want.Mode != want.Mode || len(disc.want.Reasons) != len(want.Reasons) {
		t.Errorf("disc case = %+v, want %q lasting 6 s with AC-3 and the plan %+v", disc, discFolderName, want)
	}

	if len(disc.label) > labelWidth {
		t.Errorf("label %q is longer than the column (%d)", disc.label, labelWidth)
	}
}

func TestDiscCasesRejectAManifestWithoutTheClearDVD(t *testing.T) {
	t.Parallel()

	encrypted := `{"path":"Movie (2019)/VIDEO_TS","layout":"video_ts","errorCode":"disc_encrypted"}`
	other := strings.Replace(clearDVDRow, "Movie (2019)", "Other (2000)", 1)
	broken := filepath.Join(t.TempDir(), "broken.json")

	if err := os.WriteFile(broken, []byte("{"), 0o600); err != nil {
		t.Fatalf("write %s: %v", broken, err)
	}

	tests := []struct {
		name, manifest, profile string
		wantNoCase              bool
	}{
		{name: "missing file", manifest: filepath.Join(t.TempDir(), "missing.json"), profile: corpustest.ChromiumDesktop},
		{name: "broken file", manifest: broken, profile: corpustest.ChromiumDesktop},
		{name: "no such disc", manifest: writeDiscManifest(t, other), profile: corpustest.ChromiumDesktop, wantNoCase: true},
		{name: "encrypted disc", manifest: writeDiscManifest(t, encrypted), profile: corpustest.ChromiumDesktop, wantNoCase: true},
		{name: "unknown profile", manifest: writeDiscManifest(t, clearDVDRow), profile: "safari-ios", wantNoCase: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			cases, err := discCases(test.manifest, test.profile)
			if err == nil || len(cases) != 0 || (test.wantNoCase && !errors.Is(err, errNoDiscCase)) {
				t.Errorf("discCases = %v, %v; want an error (errNoDiscCase: %t)", cases, err, test.wantNoCase)
			}
		})
	}
}

// TestSmokePlaysTheDiscThroughASession: with -disc-manifest the DVD is one more case, found
// by its folder name and checked like a transcoded file.
func TestSmokePlaysTheDiscThroughASession(t *testing.T) {
	t.Parallel()

	want, _ := corpustest.ExpectedDisc(corpustest.DiscVideoTS, corpustest.ChromiumDesktop)
	files := append(passingFiles(t), newFakeFile(discFolderName, sessionFor(want, apigen.PlaybackDeliveryHls), "DVD-INIT", "DVD-SEG"))
	summaries := passingSummaries()
	summaries[joined("DVD-INIT", "DVD-SEG")] = outputSummary{
		videoCodec: "h264", videoProfile: "High", pixelFormat: "yuv420p", audioCodec: "aac", duration: 6 * time.Second,
	}

	_, server := newFakeServer(t, files...)
	opts := testOptions(t, server, passingManifest(t))
	opts.discManifest = writeDiscManifest(t, clearDVDRow)

	var out bytes.Buffer

	if err := smoke(t.Context(), opts, testDependencies(t, summaries), &out); err != nil {
		t.Fatalf("smoke = %v, want nil; output:\n%s", err, out.String())
	}

	for _, line := range []string{"6 cases", "ok   disc " + corpustest.DiscVideoTS, "summary: 6 passed"} {
		if !strings.Contains(out.String(), line) {
			t.Errorf("output lacks %q:\n%s", line, out.String())
		}
	}
}
