package ffmpeg

import (
	"errors"
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

// testLibraryRoot returns an absolute library root for argument tests; nothing is read.
func testLibraryRoot(t *testing.T) string {
	t.Helper()

	return filepath.Join(t.TempDir(), "Library")
}

func blurayInput(root string) DiscInput {
	return DiscInput{
		LibraryRoot: root,
		DiscPath:    "Movie (2019)",
		Layout:      media.LayoutBDMV,
		Title: media.DiscTitle{
			{Path: "BDMV/STREAM/00001.m2ts", Duration: 90 * time.Second},
			{Path: "BDMV/STREAM/00002.m2ts", Duration: 1500 * time.Millisecond},
			{Path: "BDMV/STREAM/00003.m2ts"},
		},
	}
}

func dvdInput(root string) DiscInput {
	return DiscInput{
		LibraryRoot: root,
		DiscPath:    "Show/Disc 1",
		Layout:      media.LayoutVideoTS,
		Title: media.DiscTitle{
			{Path: "VIDEO_TS/VTS_01_1.VOB", Duration: time.Hour},
			{Path: "VIDEO_TS/VTS_01_2.VOB"},
		},
	}
}

func TestDiscInputRejectsUnsafeInput(t *testing.T) {
	root := testLibraryRoot(t)

	tests := []struct {
		name   string
		change func(input *DiscInput)
		want   error
	}{
		{name: "file layout", change: func(input *DiscInput) { input.Layout = media.LayoutFile }, want: domain.ErrInvalid},
		{name: "unknown layout", change: func(input *DiscInput) { input.Layout = "" }, want: domain.ErrInvalid},
		{name: "image with folder clips", change: func(input *DiscInput) { input.Layout = media.LayoutISO }, want: domain.ErrInvalid},
		{name: "relative library root", change: func(input *DiscInput) { input.LibraryRoot = "Library" }, want: domain.ErrInvalid},
		{name: "pipe in library root", change: func(input *DiscInput) { input.LibraryRoot = root + "|x" }, want: domain.ErrInvalid},
		{name: "disc path leaves the root", change: func(input *DiscInput) { input.DiscPath = "../other" }, want: domain.ErrInvalid},
		{name: "absolute disc path", change: func(input *DiscInput) { input.DiscPath = "/etc" }, want: domain.ErrInvalid},
		{name: "empty title", change: func(input *DiscInput) { input.Title = nil }, want: media.ErrDiscNoTitle},
		{name: "extent clip", change: func(input *DiscInput) { input.Title[0] = media.DiscClip{Offset: 0, Length: 2048} }, want: domain.ErrInvalid},
		{name: "clip leaves the disc", change: func(input *DiscInput) { input.Title[1].Path = "../../secret.m2ts" }, want: domain.ErrInvalid},
		{name: "absolute clip", change: func(input *DiscInput) { input.Title[1].Path = "/etc/passwd" }, want: domain.ErrInvalid},
		{name: "pipe in clip", change: func(input *DiscInput) { input.Title[1].Path = "a|file:/etc/passwd" }, want: domain.ErrInvalid},
		{name: "line break in clip", change: func(input *DiscInput) { input.Title[1].Path = "a\nfile /etc/passwd" }, want: domain.ErrInvalid},
		{name: "backslash in clip", change: func(input *DiscInput) { input.Title[1].Path = `BDMV\..\..\x.m2ts` }, want: domain.ErrInvalid},
		{name: "unclean clip", change: func(input *DiscInput) { input.Title[1].Path = "BDMV/./STREAM/00002.m2ts" }, want: domain.ErrInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := blurayInput(root)
			input.Title = slices.Clone(input.Title)
			test.change(&input)

			prepared, err := PrepareDiscInput(t.Context(), input)
			if !errors.Is(err, test.want) || prepared != nil {
				t.Fatalf("PrepareDiscInput = %v, %v, want nil, %v", prepared, err, test.want)
			}
		})
	}
}

func TestDiscInputAcceptsADiscAtTheLibraryRoot(t *testing.T) {
	root := testLibraryRoot(t)

	for _, discPath := range []string{"", "."} {
		input := dvdInput(root)
		input.DiscPath = discPath

		prepared, err := PrepareDiscInput(t.Context(), input)
		if err != nil {
			t.Fatalf("PrepareDiscInput(%q) = %v", discPath, err)
		}

		want := fileProtocol + filepath.Join(root, "VIDEO_TS", "VTS_01_1.VOB")
		if url := prepared.arguments[len(prepared.arguments)-1]; !strings.HasPrefix(url, concatProtocolPrefix+want+"|") {
			t.Errorf("input URL %q does not start with %q", url, want)
		}
	}
}

func TestPrepareDiscInputJoinsDVDClipsWithTheConcatProtocol(t *testing.T) {
	root := testLibraryRoot(t)

	prepared, err := PrepareDiscInput(t.Context(), dvdInput(root))
	if err != nil {
		t.Fatalf("PrepareDiscInput = %v", err)
	}

	clip := func(name string) string {
		return fileProtocol + filepath.Join(root, "Show", "Disc 1", "VIDEO_TS", name)
	}

	want := []string{
		"-f", "mpeg", "-format_whitelist", "mpeg", "-protocol_whitelist", "file,concat",
		"concat:" + clip("VTS_01_1.VOB") + "|" + clip("VTS_01_2.VOB"),
	}
	if !slices.Equal(prepared.arguments, want) {
		t.Errorf("arguments = %q, want %q", prepared.arguments, want)
	}

	if prepared.directory != "" {
		t.Errorf("a DVD input has the list directory %q, want none", prepared.directory)
	}

	if err := prepared.Close(); err != nil {
		t.Errorf("Close = %v", err)
	}
}

func TestPrepareDiscInputWritesAPrivateConcatListForBluray(t *testing.T) {
	temporary := useTemporaryDirectory(t)
	root := testLibraryRoot(t)

	prepared, err := PrepareDiscInput(t.Context(), blurayInput(root))
	if err != nil {
		t.Fatalf("PrepareDiscInput = %v", err)
	}

	listPath := filepath.Join(prepared.directory, discListName)
	want := []string{
		"-f", "concat", "-safe", "0", "-format_whitelist", "concat", "-protocol_whitelist", "file",
		fileProtocol + listPath,
	}
	if !slices.Equal(prepared.arguments, want) {
		t.Errorf("arguments = %q, want %q", prepared.arguments, want)
	}

	if filepath.Dir(prepared.directory) != temporary {
		t.Errorf("list directory %q is not inside %q", prepared.directory, temporary)
	}

	assertPrivate(t, prepared.directory, listPath)

	list, err := os.ReadFile(listPath) //nolint:gosec // the list the test just prepared
	if err != nil {
		t.Fatalf("read the list: %v", err)
	}

	clip := func(name string) string {
		return "file '" + fileProtocol + filepath.Join(root, "Movie (2019)", "BDMV", "STREAM", name) + "'\n" +
			"option format_whitelist mpegts\noption protocol_whitelist file\n"
	}

	wantList := "ffconcat version 1.0\n" +
		clip("00001.m2ts") + "duration 90.000000\n" +
		clip("00002.m2ts") + "duration 1.500000\n" +
		clip("00003.m2ts")
	if string(list) != wantList {
		t.Errorf("list =\n%s\nwant\n%s", list, wantList)
	}

	if err := prepared.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}

	if _, err := os.Stat(prepared.directory); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("list directory after Close: %v, want it removed", err)
	}
}

func TestConcatListQuotesSingleQuotes(t *testing.T) {
	list := concatList([]concatEntry{{url: "file:/media/It's/00001.m2ts"}}, folderClipOptions)

	if want := `file 'file:/media/It'\''s/00001.m2ts'`; !strings.Contains(list, want+"\n") {
		t.Errorf("list %q lacks %q", list, want)
	}
}

func TestDiscInputArgumentsCloseIsSafeWithoutDirectory(t *testing.T) {
	var missing *DiscInputArguments
	if err := missing.Close(); err != nil {
		t.Errorf("Close on nil = %v", err)
	}

	if err := (&DiscInputArguments{}).Close(); err != nil {
		t.Errorf("Close without a directory = %v", err)
	}
}

func TestPrepareDiscInputReportsAMissingTemporaryDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	t.Setenv("TMPDIR", missing)
	t.Setenv("TMP", missing)
	t.Setenv("TEMP", missing)

	prepared, err := PrepareDiscInput(t.Context(), blurayInput(testLibraryRoot(t)))
	if err == nil || errors.Is(err, domain.ErrInvalid) || prepared != nil {
		t.Fatalf("PrepareDiscInput = %v, %v, want an I/O error", prepared, err)
	}
}

func TestHLSArgumentsReadADiscInput(t *testing.T) {
	prepared, err := PrepareDiscInput(t.Context(), dvdInput(testLibraryRoot(t)))
	if err != nil {
		t.Fatalf("PrepareDiscInput = %v", err)
	}

	arguments, err := HLSArguments(HLSJob{
		Plan:            copyPlan(),
		Disc:            prepared,
		StartPosition:   5 * time.Second,
		OutputDirectory: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("HLSArguments = %v", err)
	}

	seek := slices.Index(arguments, "-ss")
	input := slices.Index(arguments, "-i")
	format := slices.Index(arguments, "-f")

	switch {
	case seek < 0 || format < seek || input < format:
		t.Errorf("arguments %q: want -ss before the disc input options before -i", arguments)
	case arguments[input+1] != prepared.arguments[len(prepared.arguments)-1]:
		t.Errorf("input URL = %q, want %q", arguments[input+1], prepared.arguments[len(prepared.arguments)-1])
	}
}

func TestHLSArgumentsRejectAnEmptyDiscInput(t *testing.T) {
	_, err := HLSArguments(HLSJob{Plan: copyPlan(), Disc: &DiscInputArguments{}, OutputDirectory: t.TempDir()})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("HLSArguments = %v, want %v", err, domain.ErrInvalid)
	}
}

func TestDescribeDiscUsesTheTitleDurationAndFlagsBitmapSubtitles(t *testing.T) {
	output := []byte(`{"format":{"format_name":"concat","duration":"N/A"},"streams":[
		{"index":0,"codec_type":"video","codec_name":"h264","width":1920,"height":1080},
		{"index":1,"codec_type":"audio","codec_name":"ac3","channels":6},
		{"index":2,"codec_type":"subtitle","codec_name":"hdmv_pgs_subtitle"}]}`)

	input := blurayInput(testLibraryRoot(t))

	description, err := describeDisc(output, input.Layout, input.Title)
	if err != nil {
		t.Fatalf("describeDisc = %v", err)
	}

	if description.Container != media.ContainerM2TS || description.Duration != input.Title.Duration() {
		t.Errorf("container %q, duration %s, want m2ts, %s", description.Container, description.Duration, input.Title.Duration())
	}

	if len(description.Streams) != 3 || !description.Streams[2].IsBitmapSubtitle() {
		t.Errorf("streams = %+v, want a bitmap subtitle third", description.Streams)
	}
}

func TestDescribeDiscKeepsTheProbedDurationWithoutTitleDurations(t *testing.T) {
	output := []byte(`{"format":{"format_name":"mpeg","duration":"6.005800"},"streams":[
		{"index":0,"codec_type":"video","codec_name":"mpeg2video"},
		{"index":1,"codec_type":"subtitle","codec_name":"dvd_subtitle"}]}`)

	input := dvdInput(testLibraryRoot(t))
	input.Title = media.DiscTitle{{Path: "VIDEO_TS/VTS_01_1.VOB"}}

	description, err := describeDisc(output, input.Layout, input.Title)
	if err != nil {
		t.Fatalf("describeDisc = %v", err)
	}

	if description.Container != media.ContainerVOB || description.Duration != 6005800*time.Microsecond {
		t.Errorf("container %q, duration %s, want vob, 6.0058s", description.Container, description.Duration)
	}

	if !description.Streams[1].IsBitmapSubtitle() {
		t.Errorf("DVD subtitle %+v is not flagged as bitmap", description.Streams[1])
	}
}

func TestDescribeDiscMarksUnusableOutputAsDiscUnsupported(t *testing.T) {
	_, err := describeDisc([]byte("{"), media.LayoutVideoTS, dvdInput(testLibraryRoot(t)).Title)
	if !errors.Is(err, media.ErrDiscUnsupported) || !errors.Is(err, domain.ErrUnsupported) {
		t.Errorf("describeDisc = %v, want %v", err, media.ErrDiscUnsupported)
	}
}

func TestClassifyDiscRunError(t *testing.T) {
	exit := &exitError{tool: ffprobeName, code: 1, stderr: "Unknown input format: 'concat'"}
	if err := classifyDiscRunError(exit); !errors.Is(err, media.ErrDiscUnsupported) {
		t.Errorf("exit error -> %v, want %v", err, media.ErrDiscUnsupported)
	}

	if err := classifyDiscRunError(domain.ErrUnavailable); errors.Is(err, media.ErrDiscUnsupported) ||
		!errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("start failure -> %v, want it unchanged", err)
	}
}

// copyPlan is a remux plan with one copied video and audio stream.
func copyPlan() playback.Plan {
	return playback.Plan{
		Mode: playback.ModeRemux, Delivery: playback.DeliveryHLS,
		Streams: []playback.StreamDecision{
			{Kind: media.StreamKindVideo, SourceIndex: 0, Action: playback.ActionCopy, TargetCodec: "h264"},
			{Kind: media.StreamKindAudio, SourceIndex: 1, Action: playback.ActionCopy, TargetCodec: "ac3"},
		},
	}
}

// useTemporaryDirectory points os.TempDir at a fresh directory and returns it.
func useTemporaryDirectory(t *testing.T) string {
	t.Helper()

	directory := t.TempDir()
	t.Setenv("TMPDIR", directory)
	t.Setenv("TMP", directory)
	t.Setenv("TEMP", directory)

	return directory
}

// assertPrivate checks that only the owner can read the list directory and file (unix).
func assertPrivate(t *testing.T, directory, file string) {
	t.Helper()

	if runtime.GOOS == windowsOS {
		return
	}

	for path, want := range map[string]os.FileMode{directory: 0o700, file: 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}

		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s has mode %v, want %v", path, got, want)
		}
	}
}
