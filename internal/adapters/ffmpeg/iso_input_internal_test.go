package ffmpeg

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

const (
	testImageFolder   = "Movies"
	testImagePath     = "Movies/Film.iso"
	testBlurayClipOne = "BDMV/STREAM/00001.M2TS"
	testBlurayClipTwo = "BDMV/STREAM/00002.M2TS"
	testDVDVOBOne     = "VIDEO_TS/VTS_01_1.VOB"
	testDVDVOBTwo     = "VIDEO_TS/VTS_01_2.VOB"
)

// imageInputTest is a library root holding one generated image.
type imageInputTest struct {
	root    string
	offsets map[string]int64
	files   map[string]string
}

// newImageInputTest writes an image of files (path to content) to testImagePath in a new
// library root.
func newImageInputTest(t *testing.T, files map[string]string) imageInputTest {
	t.Helper()

	isoFiles := make([]TestISOFile, 0, len(files))
	for name, content := range files {
		isoFiles = append(isoFiles, TestISOFile{Path: name, Data: []byte(content)})
	}

	image, offsets := BuildTestISO(isoFiles)
	root := t.TempDir()
	writeImage(t, root, image)

	return imageInputTest{root: root, offsets: offsets, files: files}
}

func writeImage(t *testing.T, root string, image []byte) {
	t.Helper()

	if err := os.MkdirAll(filepath.Join(root, testImageFolder), 0o700); err != nil {
		t.Fatalf("create the image folder: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(testImagePath)), image, 0o600); err != nil {
		t.Fatalf("write the image: %v", err)
	}
}

// clip is the title clip of the image file name.
func (test imageInputTest) clip(name string, start, duration time.Duration) media.DiscClip {
	return media.DiscClip{
		Offset: test.offsets[name], Length: int64(len(test.files[name])), Start: start, Duration: duration,
	}
}

func (test imageInputTest) input(title ...media.DiscClip) DiscInput {
	return DiscInput{LibraryRoot: test.root, DiscPath: testImagePath, Layout: media.LayoutISO, Title: title}
}

func newBlurayImageTest(t *testing.T) imageInputTest {
	t.Helper()

	return newImageInputTest(t, map[string]string{
		testBlurayClipOne: "first transport stream", testBlurayClipTwo: "second transport stream",
		"BDMV/PLAYLIST/00000.MPLS": "playlist",
	})
}

func newDVDImageTest(t *testing.T) imageInputTest {
	t.Helper()

	return newImageInputTest(t, map[string]string{
		testDVDVOBOne: "program stream, part one; ", testDVDVOBTwo: "part two",
		"VIDEO_TS/VTS_01_0.IFO": "info",
	})
}

func prepareOrFail(t *testing.T, input DiscInput) *DiscInputArguments {
	t.Helper()

	prepared, err := PrepareDiscInput(t.Context(), input)
	if err != nil {
		t.Fatalf("PrepareDiscInput = %v", err)
	}

	t.Cleanup(func() { _ = prepared.Close() })

	return prepared
}

// subfile is the expected subfile URL of the image file name.
func (test imageInputTest) subfile(name string) string {
	start := test.offsets[name]
	end := start + int64(len(test.files[name]))

	return "subfile,,start," + strconv.FormatInt(start, 10) + ",end," + strconv.FormatInt(end, 10) + ",," +
		":file:" + filepath.Join(test.root, filepath.FromSlash(testImagePath))
}

func TestPrepareDiscInputReadsABlurayImageThroughAConcatList(t *testing.T) {
	temporary := useTemporaryDirectory(t)
	test := newBlurayImageTest(t)

	prepared := prepareOrFail(t, test.input(
		test.clip(testBlurayClipOne, 1500*time.Millisecond, 90*time.Second),
		test.clip(testBlurayClipTwo, 0, 0),
	))

	listPath := filepath.Join(prepared.directory, discListName)
	want := []string{
		"-f", "concat", "-safe", "0", "-format_whitelist", "concat", "-protocol_whitelist", "file", fileProtocol + listPath,
	}
	if !slices.Equal(prepared.arguments, want) || prepared.layout != media.LayoutBDMV {
		t.Errorf("arguments %q, layout %q, want %q, bdmv", prepared.arguments, prepared.layout, want)
	}

	list, err := os.ReadFile(listPath) //nolint:gosec // the list the test just prepared
	if err != nil {
		t.Fatalf("read the list: %v", err)
	}

	entry := func(name string) string {
		return "file '" + test.subfile(name) + "'\noption format_whitelist mpegts\noption protocol_whitelist file,subfile\n"
	}
	wantList := discListHeader +
		entry(testBlurayClipOne) + "inpoint 1.500000\noutpoint 91.500000\nduration 90.000000\n" +
		entry(testBlurayClipTwo)

	if string(list) != wantList {
		t.Errorf("list =\n%s\nwant\n%s", list, wantList)
	}

	if err := prepared.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}

	assertEmptyDirectory(t, temporary)
}

func TestPrepareDiscInputJoinsADVDImageIntoOneProgramStream(t *testing.T) {
	test := newDVDImageTest(t)

	prepared := prepareOrFail(t, test.input(
		test.clip(testDVDVOBOne, 0, time.Minute),
		test.clip(testDVDVOBTwo, 0, 0),
	))

	want := []string{
		"-f", "mpeg", "-format_whitelist", "mpeg", "-protocol_whitelist", "file,subfile,concat",
		"concat:" + test.subfile(testDVDVOBOne) + "|" + test.subfile(testDVDVOBTwo),
	}
	if !slices.Equal(prepared.arguments, want) || prepared.layout != media.LayoutVideoTS || prepared.directory != "" {
		t.Errorf("arguments %q, layout %q, want %q, video_ts", prepared.arguments, prepared.layout, want)
	}
}

func TestPrepareDiscInputRejectsBadImages(t *testing.T) {
	image := newBlurayImageTest(t)
	clip := image.clip(testBlurayClipOne, 0, time.Second)

	tests := []struct {
		name   string
		change func(t *testing.T, input *DiscInput)
		want   error
	}{
		{name: "folder clip in an image", change: func(_ *testing.T, input *DiscInput) { input.Title[0] = media.DiscClip{Path: "BDMV/STREAM/00001.m2ts"} }, want: domain.ErrInvalid},
		{name: "image path leaves the root", change: func(_ *testing.T, input *DiscInput) { input.DiscPath = "../Film.iso" }, want: domain.ErrInvalid},
		{name: "image at the library root", change: func(_ *testing.T, input *DiscInput) { input.DiscPath = "." }, want: domain.ErrInvalid},
		{name: "no image path", change: func(_ *testing.T, input *DiscInput) { input.DiscPath = "" }, want: domain.ErrInvalid},
		{name: "missing image", change: func(_ *testing.T, input *DiscInput) { input.DiscPath = "Movies/Other.iso" }, want: fs.ErrNotExist},
		{name: "extent beyond the image", change: func(_ *testing.T, input *DiscInput) { input.Title[0].Offset = 1 << 40 }, want: domain.ErrInvalid},
		{name: "extent ends beyond the image", change: func(_ *testing.T, input *DiscInput) { input.Title[0].Length = 1 << 40 }, want: domain.ErrInvalid},
		{
			name: "no disc file system",
			change: func(t *testing.T, input *DiscInput) {
				input.LibraryRoot = t.TempDir()
				writeImage(t, input.LibraryRoot, make([]byte, 64*testISOSector))
				input.Title[0] = media.DiscClip{Length: testISOSector}
			},
			want: media.ErrDiscUnsupported,
		},
		{
			name: "no disc structure",
			change: func(t *testing.T, input *DiscInput) {
				data, _ := BuildTestISO([]TestISOFile{{Path: "README.TXT", Data: []byte("not a disc")}})
				input.LibraryRoot = t.TempDir()
				writeImage(t, input.LibraryRoot, data)
				input.Title[0] = media.DiscClip{Length: testISOSector}
			},
			want: media.ErrDiscUnsupported,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := image.input(clip)
			test.change(t, &input)

			prepared, err := PrepareDiscInput(t.Context(), input)
			if !errors.Is(err, test.want) || prepared != nil {
				t.Fatalf("PrepareDiscInput = %v, %v, want nil, %v", prepared, err, test.want)
			}
		})
	}
}

// TestPrepareDiscInputDoesNotFollowASymlinkOutOfTheRoot: the image is opened inside the
// library root, so a link that points outside it is refused.
func TestPrepareDiscInputDoesNotFollowASymlinkOutOfTheRoot(t *testing.T) {
	outside := newBlurayImageTest(t)
	root := t.TempDir()

	if err := os.MkdirAll(filepath.Join(root, testImageFolder), 0o700); err != nil {
		t.Fatalf("create the folder: %v", err)
	}

	link := filepath.Join(root, filepath.FromSlash(testImagePath))
	if err := os.Symlink(filepath.Join(outside.root, filepath.FromSlash(testImagePath)), link); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}

	input := outside.input(outside.clip(testBlurayClipOne, 0, time.Second))
	input.LibraryRoot = root

	if prepared, err := PrepareDiscInput(t.Context(), input); err == nil {
		_ = prepared.Close()
		t.Fatal("PrepareDiscInput followed a symlink out of the library root")
	}
}

func TestPrepareDiscInputHonoursACancelledContext(t *testing.T) {
	test := newBlurayImageTest(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	prepared, err := PrepareDiscInput(ctx, test.input(test.clip(testBlurayClipOne, 0, time.Second)))
	if err == nil || prepared != nil {
		t.Fatalf("PrepareDiscInput = %v, %v, want an error", prepared, err)
	}

	if errors.Is(err, media.ErrDiscUnsupported) {
		t.Errorf("a cancelled preparation is reported as %v", err)
	}
}

func TestConcatListCutsClipsToTheirSpan(t *testing.T) {
	list := concatList([]concatEntry{
		{url: "file:/a.m2ts", start: 2 * time.Second, duration: 3 * time.Second},
		{url: "file:/b.m2ts", start: 4 * time.Second},
		{url: "file:/c.m2ts", duration: time.Second},
	}, folderClipOptions)

	want := discListHeader +
		"file 'file:/a.m2ts'\n" + folderClipOptions + "inpoint 2.000000\noutpoint 5.000000\nduration 3.000000\n" +
		"file 'file:/b.m2ts'\n" + folderClipOptions +
		"file 'file:/c.m2ts'\n" + folderClipOptions + "duration 1.000000\n"
	if list != want {
		t.Errorf("list =\n%s\nwant\n%s", list, want)
	}

	if strings.Count(list, "inpoint") != 1 {
		t.Errorf("a clip without a duration must not be cut: %s", list)
	}
}
