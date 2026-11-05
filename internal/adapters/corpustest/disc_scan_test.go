package corpustest_test

import (
	"os/exec"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/disc"
	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/adapters/uuidgen"
	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/media"
)

// TestCorpusDiscLibraryScans scans the disc library with the real walker, resolvers and prober
// (M3-16h): every disc of the manifest becomes one catalogued file with its layout; clear
// discs carry the main title's duration and streams, the stand-ins their disc error code.
func TestCorpusDiscLibraryScans(t *testing.T) {
	t.Parallel()

	corpus := requireDiscCorpus(t)

	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Fatalf("ffprobe disappeared from PATH: %v", err)
	}

	graph := newCorpusGraph(t, corpusEnvironment{ffmpegPath: corpus.tools.ffmpegPath, ffprobePath: ffprobePath, root: corpus.root})
	files := scanDiscs(t, graph)

	reader := sqlite.NewCatalogReader(graph.database)

	for _, row := range corpus.discs {
		t.Run(row.Title, func(t *testing.T) {
			fileID, found := files[row.Path]
			if !found {
				t.Fatalf("the disc %q is not catalogued (catalogued: %v)", row.Path, files)
			}

			playable, err := reader.PlayableFile(t.Context(), fileID)
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
