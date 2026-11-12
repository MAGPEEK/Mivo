package corpustest_test

import (
	"os/exec"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/corpustest"
	"github.com/magpeek/mivo/internal/adapters/disc"
	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/adapters/uuidgen"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/domain/playback/playbacktest"
)

// TestCorpusDVDSessionPlays plays the DVD main title through the session API (M3-16k): the
// scan catalogues the disc, a session for each reference profile gets the plan of
// corpustest.ExpectedDisc and transcodes it via HLS, and the finished output lasts the whole
// title.
func TestCorpusDVDSessionPlays(t *testing.T) {
	t.Parallel()

	corpus := requireDiscCorpus(t)
	requireTranscoding(t, corpus.tools.ffmpegPath)

	graph := newDiscGraph(t, corpus)
	row := corpus.disc(t, corpustest.DiscVideoTS)

	fileID, found := scanDiscs(t, graph)[row.Path]
	if !found {
		t.Fatalf("the disc %q is not catalogued", row.Path)
	}

	service := graph.newPlaybackService(t, playbacktest.FullCapabilities())

	for _, profile := range []playback.ClientProfile{playbacktest.ChromiumDesktop(), playbacktest.FirefoxDesktop()} {
		t.Run(profile.Name, func(t *testing.T) {
			want, found := corpustest.ExpectedDisc(row.Path, profile.Name)
			if !found {
				t.Fatalf("corpustest.ExpectedDisc has no plan of %s for %s", row.Path, profile.Name)
			}

			session := startSession(t, service, appplayback.StartRequest{FileID: fileID, Profile: profile})
			assertPlan(t, session.Plan, want)
			waitForEnd(t, service, session.ID)

			output := graph.probeOutput(t, service, session.ID)
			if (output.duration - row.duration()).Abs() > discDurationSlack {
				t.Errorf("the output lasts %s, want %s", output.duration, row.duration())
			}

			if output.videoCodec != "h264" || output.audioCodec != "aac" {
				t.Errorf("output video %q, audio %q, want h264 and aac", output.videoCodec, output.audioCodec)
			}
		})
	}
}

// newDiscGraph wires the server graph on the disc library.
func newDiscGraph(t *testing.T, corpus discCorpus) *corpusGraph {
	t.Helper()

	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Fatalf("ffprobe disappeared from PATH: %v", err)
	}

	return newCorpusGraph(t, corpusEnvironment{ffmpegPath: corpus.tools.ffmpegPath, ffprobePath: ffprobePath, root: corpus.root})
}

// TestCorpusDiscLibraryScans scans the disc library with the real walker, resolvers and prober
// (M3-16h): every disc of the manifest becomes one catalogued file with its layout; clear
// discs carry the main title's duration and streams, the stand-ins their disc error code.
func TestCorpusDiscLibraryScans(t *testing.T) {
	t.Parallel()

	corpus := requireDiscCorpus(t)
	graph := newDiscGraph(t, corpus)
	files := scanDiscs(t, graph)

	reader := sqlite.NewCatalogReader(graph.database)

	for _, row := range corpus.discs {
		t.Run(row.Title, func(t *testing.T) {
			fileID, found := files[row.Path]
			if !found {
				t.Fatalf("the disc %q is not catalogued (catalogued: %v)", row.Path, files)
			}

			playable, err := reader.PlayableFile(t.Context(), fileID, access.Unrestricted())
			if err != nil {
				t.Fatalf("PlayableFile: %v", err)
			}

			assertScannedDisc(t, row, playable.File)
		})
	}

	if len(files) != len(corpus.discs) {
		t.Errorf("catalogued %d files, want one per disc (%d): %v", len(files), len(corpus.discs), files)
	}
}

// scanDiscs scans the graph's library with the disc ports and returns the file IDs by path.
func scanDiscs(t *testing.T, graph *corpusGraph) map[string]string {
	t.Helper()

	catalog := sqlite.NewLibraryCatalog(graph.database, time.Now)

	service, err := scan.NewService(scan.Dependencies{
		Libraries: graph.libraries(),
		Walker:    graph.files, Resolver: graph.files, Prober: graph.prober, CatalogV2: catalog,
		Discs: scan.DiscPorts{Directories: graph.files, Titles: disc.NewResolver(), Prober: graph.prober},
		IDs:   uuidgen.New(), Clock: time.Now, Logger: graph.logger,
	})
	if err != nil {
		t.Fatalf("scan.NewService: %v", err)
	}

	defer closeWithTimeout(t, "scan service", service.Close)

	if _, err := service.Start(t.Context(), graph.libraryID); err != nil {
		t.Fatalf("start the scan: %v", err)
	}

	if status := waitForScan(t, service, graph.libraryID); status.State != scan.StateIdle {
		t.Fatalf("scan ended with state %s, code %q, counts %+v", status.State, status.ErrorCode, status.Counts)
	}

	stored, err := catalog.PlacedFiles(t.Context(), graph.libraryID)
	if err != nil {
		t.Fatalf("list the catalogued files: %v", err)
	}

	fileIDs := make(map[string]string, len(stored))
	for _, file := range stored {
		fileIDs[file.State.RelativePath] = file.ID
	}

	return fileIDs
}

func assertScannedDisc(t *testing.T, row discRow, file media.File) {
	t.Helper()

	if file.Layout != row.layout() {
		t.Errorf("layout = %q, want %q", file.Layout, row.layout())
	}

	if row.ErrorCode != "" {
		if string(file.ProbeError) != row.ErrorCode {
			t.Errorf("probe error = %q, want %q", file.ProbeError, row.ErrorCode)
		}

		return
	}

	if !file.Probed() {
		t.Fatalf("probe error = %q, want a probed title", file.ProbeError)
	}

	if (file.Description.Duration - row.duration()).Abs() > discDurationSlack {
		t.Errorf("duration = %s, want %s", file.Description.Duration, row.duration())
	}

	if _, hasVideo := file.Description.PrimaryVideo(); !hasVideo {
		t.Errorf("streams = %+v, want a video stream", file.Description.Streams)
	}
}
