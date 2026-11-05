package corpustest_test

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/disc"
	"github.com/magpeek/mivo/internal/adapters/disc/iso"
	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/adapters/fs/libraryfs"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/domain/playback/playbacktest"
)

const (
	// discCasePrefix starts the names of the disc source and assembly cases.
	discCasePrefix = "library-discs-"
	// discDurationSlack allows for the encoder delay and the last frame of a probed title.
	discDurationSlack = 600 * time.Millisecond
	// discPlaybackTimeout bounds producing one title (a few seconds of SD video).
	discPlaybackTimeout = 2 * time.Minute
	discSessionID       = "disc-corpus"
)

// discManifest is the discs part of testdata/library/manifest.json (task M3-16i).
type discManifest struct {
	Path  string    `json:"path"`
	Items []discRow `json:"items"`
}

type discRow struct {
	Title     string `json:"title"`
	Path      string `json:"path"`
	Layout    string `json:"layout"`
	MainTitle struct {
		Clips           []string `json:"clips"`
		DurationSeconds float64  `json:"durationSeconds"`
	} `json:"mainTitle"`
	ErrorCode string          `json:"errorCode"`
	Streams   *discStreamsRow `json:"streams"`
}

type discStreamsRow struct {
	Video     string   `json:"video"`
	Width     int      `json:"width"`
	Height    int      `json:"height"`
	Audio     []string `json:"audio"`
	Subtitles []string `json:"subtitles"`
}

func (row discRow) layout() media.Layout { return media.Layout(row.Layout) }

func (row discRow) duration() time.Duration {
	return time.Duration(row.MainTitle.DurationSeconds * float64(time.Second))
}

// discCorpus is the generated disc library and the adapters under test.
type discCorpus struct {
	root   string
	discs  []discRow
	tools  discTools
	logger *slog.Logger
}

type discTools struct {
	ffmpegPath string
	prober     *ffmpeg.Prober
}

// requireDiscCorpus reads the disc part of the library manifest, or skips under -short,
// without FFmpeg or without the generated corpus. Every disc case must have been generated.
func requireDiscCorpus(t *testing.T) discCorpus {
	t.Helper()

	readers, libraryRoot := requireMusicCorpus(t)
	manifest := readLibraryManifest(t, libraryRoot)

	if len(manifest.Discs.Items) == 0 {
		t.Fatal("the library manifest lists no disc; regenerate it with `go run ./scripts/gen-testmedia`")
	}

	for _, result := range manifest.Cases {
		if strings.HasPrefix(result.Name, discCasePrefix) && result.Status != "generated" {
			t.Fatalf("disc case %s is %s: %s", result.Name, result.Status, result.Reason)
		}
	}

	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatalf("ffmpeg disappeared from PATH: %v", err)
	}

	return discCorpus{
		root:   filepath.Join(libraryRoot, filepath.FromSlash(manifest.Discs.Path)),
		discs:  manifest.Discs.Items,
		tools:  discTools{ffmpegPath: ffmpegPath, prober: readers.prober},
		logger: slog.New(slog.DiscardHandler),
	}
}

// TestCorpusDiscsAreDetected walks the disc library like a scan: every disc of the manifest
// is reported once with its layout, and no clip inside a disc is listed as a video.
func TestCorpusDiscsAreDetected(t *testing.T) {
	t.Parallel()

	corpus := requireDiscCorpus(t)

	walker, err := libraryfs.New(corpus.logger)
	if err != nil {
		t.Fatalf("libraryfs.New: %v", err)
	}

	found := map[string]media.Layout{}

	skipped, err := walker.Walk(t.Context(), corpus.root, library.KindMovies, func(state library.FileState) error {
		found[state.RelativePath] = state.Layout

		return nil
	})
	if err != nil || len(skipped) > 0 {
		t.Fatalf("Walk = %v, skipped %v", err, skipped)
	}

	want := map[string]media.Layout{}
	for _, row := range corpus.discs {
		want[row.Path] = row.layout()
	}

	for relativePath, layout := range found {
		wantLayout, isDisc := want[relativePath]
		if !isDisc {
			t.Errorf("the walk reports %q (layout %q), which is no disc of the manifest", relativePath, layout)

			continue
		}

		if layout != wantLayout {
			t.Errorf("the walk reports %q with layout %q, the manifest %q", relativePath, layout, wantLayout)
		}
	}

	for relativePath := range want {
		if _, ok := found[relativePath]; !ok {
			t.Errorf("the walk does not report the disc %q", relativePath)
		}
	}
}

// TestCorpusDiscTitlesAreSelected resolves every disc: clear discs give the manifest's main
// title (clips and IFO/MPLS duration), the stand-ins fail with their error code.
func TestCorpusDiscTitlesAreSelected(t *testing.T) {
	t.Parallel()

	corpus := requireDiscCorpus(t)
	resolver := disc.NewResolver()

	for _, row := range corpus.discs {
		t.Run(row.Title, func(t *testing.T) {
			t.Parallel()

			title, err := resolver.Resolve(t.Context(), corpus.path(row), row.layout())
			if row.ErrorCode != "" {
				if code, _ := media.DiscErrorCode(err); code != row.ErrorCode {
					t.Fatalf("Resolve = %v, want the code %s", err, row.ErrorCode)
				}

				return
			}

			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}

			corpus.assertSelectedTitle(t, row, title)
		})
	}
}

// assertSelectedTitle compares a resolved title with the manifest: duration, and the clip
// paths of a folder or the extents of an image.
func (corpus discCorpus) assertSelectedTitle(t *testing.T, row discRow, title media.DiscTitle) {
	t.Helper()

	if title.Duration() != row.duration() {
		t.Errorf("the main title lasts %s, want %s", title.Duration(), row.duration())
	}

	if row.layout() == media.LayoutISO {
		assertImageTitle(t, corpus.path(row), title, row.MainTitle.Clips)

		return
	}

	if got := clipPaths(title); !slices.Equal(got, row.MainTitle.Clips) {
		t.Errorf("the main title plays %q, want %q", got, row.MainTitle.Clips)
	}
}

func (corpus discCorpus) path(row discRow) string {
	return filepath.Join(corpus.root, filepath.FromSlash(row.Path))
}

func clipPaths(title media.DiscTitle) []string {
	paths := make([]string, 0, len(title))
	for _, clip := range title {
		paths = append(paths, clip.Path)
	}

	return paths
}

// assertImageTitle checks that the title's image extents hold exactly the bytes of the clip
// files inside the image, in order.
func assertImageTitle(t *testing.T, imagePath string, title media.DiscTitle, clips []string) {
	t.Helper()

	imageFile, err := os.Open(imagePath) //nolint:gosec // corpus file
	if err != nil {
		t.Fatalf("open the image: %v", err)
	}

	defer func() { _ = imageFile.Close() }()

	info, err := imageFile.Stat()
	if err != nil {
		t.Fatalf("stat the image: %v", err)
	}

	image, err := iso.Open(t.Context(), imageFile, info.Size())
	if err != nil {
		t.Fatalf("iso.Open: %v", err)
	}

	var want, got bytes.Buffer

	for _, clip := range clips {
		data, err := fs.ReadFile(image, clip)
		if err != nil {
			t.Fatalf("read %s inside the image: %v", clip, err)
		}

		want.Write(data)
	}

	for _, extent := range title {
		if _, err := io.Copy(&got, io.NewSectionReader(imageFile, extent.Offset, extent.Length)); err != nil {
			t.Fatalf("read the extent at %d: %v", extent.Offset, err)
		}
	}

	if !bytes.Equal(got.Bytes(), want.Bytes()) {
		t.Errorf("the title extents hold %d bytes that differ from the %d bytes of %q", got.Len(), want.Len(), clips)
	}
}

// TestCorpusDiscTitlesProbe probes the main title of every clear disc, folders and ISO images
// (read through subfile ranges, M3-16g): the duration and the video, audio and bitmap
// subtitle streams of the manifest.
func TestCorpusDiscTitlesProbe(t *testing.T) {
	t.Parallel()

	corpus := requireDiscCorpus(t)

	for _, row := range corpus.playableDiscs() {
		t.Run(row.Title, func(t *testing.T) {
			t.Parallel()

			description, err := corpus.tools.prober.ProbeDisc(t.Context(), corpus.discInput(t, row))
			if err != nil {
				t.Fatalf("ProbeDisc: %v", err)
			}

			if (description.Duration - row.duration()).Abs() > discDurationSlack {
				t.Errorf("the title lasts %s, want %s", description.Duration, row.duration())
			}

			assertDiscStreams(t, description, *row.Streams)
		})
	}
}

// playableDiscs are the clear discs: VIDEO_TS and BDMV folders and ISO images.
func (corpus discCorpus) playableDiscs() []discRow {
	var rows []discRow

	for _, row := range corpus.discs {
		if row.ErrorCode == "" {
			rows = append(rows, row)
		}
	}

	return rows
}

// discInput resolves the title of a disc folder or image into the FFmpeg input of its clips.
func (corpus discCorpus) discInput(t *testing.T, row discRow) ffmpeg.DiscInput {
	t.Helper()

	title, err := disc.NewResolver().Resolve(t.Context(), corpus.path(row), row.layout())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	return ffmpeg.DiscInput{LibraryRoot: corpus.root, DiscPath: row.Path, Layout: row.layout(), Title: title}
}

func assertDiscStreams(t *testing.T, description media.Description, want discStreamsRow) {
	t.Helper()

	var video media.Stream

	var audio, subtitles []string

	for _, stream := range description.Streams {
		switch stream.Kind {
		case media.StreamKindVideo:
			video = stream
		case media.StreamKindAudio:
			audio = append(audio, stream.Codec)
		case media.StreamKindSubtitle:
			subtitles = append(subtitles, stream.Codec)
		case media.StreamKindOther:
			// Navigation and data streams are not part of the manifest.
		}
	}

	if video.Codec != want.Video || video.Width != want.Width || video.Height != want.Height {
		t.Errorf("video %s %dx%d, want %s %dx%d", video.Codec, video.Width, video.Height, want.Video, want.Width, want.Height)
	}

	if !slices.Equal(audio, want.Audio) || !slices.Equal(subtitles, want.Subtitles) {
		t.Errorf("audio %q, subtitles %q, want %q, %q", audio, subtitles, want.Audio, want.Subtitles)
	}
}

// TestCorpusDVDTitlePlays decides the DVD main title for the Chromium reference profile and
// produces it as HLS through the FFmpeg producer, the way a session does: MPEG-2 and AC-3 are
// transcoded, and the joined output lasts the whole title. The session API itself reaches
// discs with the scan and composition-root wiring (M3-16h, M3-16k).
func TestCorpusDVDTitlePlays(t *testing.T) {
	t.Parallel()

	corpus := requireDiscCorpus(t)
	requireTranscoding(t, corpus.tools.ffmpegPath)

	row := corpus.firstDisc(t, media.LayoutVideoTS)
	input := corpus.discInput(t, row)

	description, err := corpus.tools.prober.ProbeDisc(t.Context(), input)
	if err != nil {
		t.Fatalf("ProbeDisc: %v", err)
	}

	plan, err := playback.Decide(playback.Input{
		Description: description, Profile: playbacktest.ChromiumDesktop(), Capabilities: playbacktest.FullCapabilities(),
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}

	if plan.Mode != playback.ModeTranscode || plan.Delivery != playback.DeliveryHLS {
		t.Fatalf("plan %s via %s, want a transcode via HLS", plan.Mode, plan.Delivery)
	}

	outputDir := corpus.produce(t, appplayback.Job{
		SessionID: discSessionID, Plan: plan, LibraryRoot: corpus.root, RelativePath: row.Path,
		InputPath: corpus.path(row), Layout: input.Layout, DiscTitle: input.Title,
	})

	output, err := corpus.tools.prober.Probe(t.Context(), joinOutput(t, outputDir, finishedSegments(t, outputDir)))
	if err != nil {
		t.Fatalf("probe the output: %v", err)
	}

	if (output.Duration - row.duration()).Abs() > discDurationSlack {
		t.Errorf("the output lasts %s, want %s", output.Duration, row.duration())
	}
}

func (corpus discCorpus) firstDisc(t *testing.T, layout media.Layout) discRow {
	t.Helper()

	for _, row := range corpus.playableDiscs() {
		if row.layout() == layout {
			return row
		}
	}

	t.Fatalf("the manifest lists no playable %s disc", layout)

	return discRow{}
}

// produce runs the job to its end and returns the output directory.
func (corpus discCorpus) produce(t *testing.T, job appplayback.Job) string {
	t.Helper()

	files, err := libraryfs.New(corpus.logger)
	if err != nil {
		t.Fatalf("libraryfs.New: %v", err)
	}

	producer, err := ffmpeg.NewProducer(ffmpeg.ProducerOptions{FFmpegPath: corpus.tools.ffmpegPath, Inputs: files, Logger: corpus.logger})
	if err != nil {
		t.Fatalf("NewProducer: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), discPlaybackTimeout)
	defer cancel()

	job.OutputDir = t.TempDir()

	production, err := producer.Start(ctx, job)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	defer production.Stop()

	select {
	case <-production.Done():
	case <-ctx.Done():
		t.Fatalf("the production did not end within %s", discPlaybackTimeout)
	}

	if err := production.Ready(ctx); err != nil {
		t.Fatalf("the production ended with %v", err)
	}

	return job.OutputDir
}

// TestCorpusDiscManifestPathsAreClean guards the manifest shape the other tests rely on.
func TestCorpusDiscManifestPathsAreClean(t *testing.T) {
	t.Parallel()

	corpus := requireDiscCorpus(t)

	for _, row := range corpus.discs {
		if path.Clean(row.Path) != row.Path || !row.layout().IsDisc() || row.duration() <= 0 ||
			(row.ErrorCode == "") != (row.Streams != nil) {
			t.Errorf("disc %q: unexpected manifest row %+v", row.Title, row)
		}
	}
}
