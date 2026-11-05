package ffmpeg

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

const (
	testDiscFolder   = "Movie (2019)"
	testClipOne      = "BDMV/STREAM/00001.m2ts"
	testClipTwo      = "BDMV/STREAM/00002.m2ts"
	testClipContents = "transport stream"
)

// newDiscProducerTest is a producer test whose job plays a two-clip Blu-ray folder inside the
// test library; os.TempDir points at a fresh directory, so the list's lifecycle is visible.
func newDiscProducerTest(t *testing.T, mode string) (producerTest, string) {
	t.Helper()

	test := newProducerTest(t, mode)
	temporary := useTemporaryDirectory(t)

	for _, clip := range []string{testClipOne, testClipTwo} {
		clipPath := filepath.Join(test.job.LibraryRoot, testDiscFolder, filepath.FromSlash(clip))
		if err := os.MkdirAll(filepath.Dir(clipPath), 0o700); err != nil {
			t.Fatalf("create the disc: %v", err)
		}

		writeTestFile(t, filepath.Dir(clipPath), filepath.Base(clipPath), testClipContents)
	}

	test.job.Layout = media.LayoutBDMV
	test.job.RelativePath = testDiscFolder
	test.job.InputPath = filepath.Join(test.job.LibraryRoot, testDiscFolder)
	test.job.DiscTitle = media.DiscTitle{{Path: testClipOne, Duration: 2 * time.Second}, {Path: testClipTwo}}
	test.job.StartPosition = 3 * time.Second

	return test, temporary
}

func TestProducerPlaysAFolderDiscThroughAConcatList(t *testing.T) {
	test, temporary := newDiscProducerTest(t, fakeModeHLSComplete)
	argumentsPath := filepath.Join(t.TempDir(), "arguments.jsonl")
	t.Setenv(fakeArgumentsVariable, argumentsPath)

	production := test.start(t.Context(), t)
	awaitDone(t, production)

	if err := production.Ready(t.Context()); err != nil {
		t.Fatalf("Ready after a complete run = %v, want nil", err)
	}

	// The fake copies its input, the concat list, into the segment.
	list, err := os.ReadFile(filepath.Join(test.job.OutputDir, fakeSegmentName))
	if err != nil {
		t.Fatalf("read the segment: %v", err)
	}

	wantClips := []string{
		filepath.Join(test.job.LibraryRoot, testDiscFolder, "BDMV", "STREAM", "00001.m2ts"),
		filepath.Join(test.job.LibraryRoot, testDiscFolder, "BDMV", "STREAM", "00002.m2ts"),
	}
	if runtime.GOOS == "linux" {
		wantClips = []string{"/proc/self/fd/3", "/proc/self/fd/4"}
	}

	for _, clip := range wantClips {
		if !strings.Contains(string(list), "file '"+fileProtocol+clip+"'\n") {
			t.Errorf("list %q does not name %q", list, clip)
		}
	}

	calls := readRecordedCalls(t, argumentsPath)
	if len(calls) != 1 {
		t.Fatalf("recorded %d calls, want 1", len(calls))
	}

	seek, format := slices.Index(calls[0], "-ss"), slices.Index(calls[0], "concat")
	if seek < 0 || format < seek || calls[0][seek+1] != "3.000000" {
		t.Errorf("arguments %q: want -ss 3 before the concat input", calls[0])
	}

	assertEmptyDirectory(t, temporary)
	assertNoWarnings(t, test.logs)
}

func TestProducerRemovesTheDiscListOnStop(t *testing.T) {
	test, temporary := newDiscProducerTest(t, fakeModeHLSLive)

	production := test.start(t.Context(), t)
	readyOrFailInternal(t, production.Ready(t.Context()))

	if entries, _ := os.ReadDir(temporary); len(entries) != 1 {
		t.Fatalf("temporary directory holds %v while FFmpeg runs, want one list directory", entries)
	}

	production.Stop()
	assertEmptyDirectory(t, temporary)
}

func TestProducerDiscStartFailures(t *testing.T) {
	tests := []struct {
		name      string
		linuxOnly bool
		change    func(t *testing.T, test *producerTest)
		want      error
	}{
		{
			name:   "unsafe clip",
			change: func(_ *testing.T, test *producerTest) { test.job.DiscTitle[1].Path = "../../../etc/passwd" },
			want:   domain.ErrInvalid,
		},
		{
			name:   "direct play plan",
			change: func(_ *testing.T, test *producerTest) { test.job.Plan.Mode = playback.ModeDirectPlay },
			want:   domain.ErrInvalid,
		},
		{
			name: "missing executable",
			change: func(t *testing.T, test *producerTest) {
				test.producer.ffmpeg.executable = filepath.Join(t.TempDir(), "no-such-ffmpeg")
			},
			want: domain.ErrUnavailable,
		},
		{
			name:      "clip missing in the library",
			linuxOnly: true,
			change: func(_ *testing.T, test *producerTest) {
				test.job.DiscTitle[1].Path = "BDMV/STREAM/00009.m2ts"
			},
			want: fs.ErrNotExist,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.linuxOnly && runtime.GOOS != "linux" {
				t.Skip("the producer opens the clips itself only on Linux")
			}

			setup, temporary := newDiscProducerTest(t, fakeModeHLSLive)
			test.change(t, &setup)

			production, err := setup.producer.Start(t.Context(), setup.job)
			if !errors.Is(err, test.want) || production != nil {
				t.Fatalf("Start = %v, %v, want nil, %v", production, err, test.want)
			}

			assertEmptyDirectory(t, temporary)
		})
	}
}

func TestProducerRefusesAnImageAsFolderDisc(t *testing.T) {
	setup, _ := newDiscProducerTest(t, fakeModeHLSLive)
	setup.job.Layout = media.LayoutISO

	if _, err := setup.producer.Start(t.Context(), setup.job); !errors.Is(err, media.ErrDiscUnsupported) {
		t.Errorf("Start = %v, want %v", err, media.ErrDiscUnsupported)
	}
}

func readyOrFailInternal(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("Ready = %v", err)
	}
}

func assertEmptyDirectory(t *testing.T, directory string) {
	t.Helper()

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read %s: %v", directory, err)
	}

	if len(entries) > 0 {
		t.Errorf("%s still holds %v", directory, entries)
	}
}
