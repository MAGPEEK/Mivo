package scan_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

// newMetadataHarness wires the v2 scan to the real MetadataRefresher and a scripted NFO.
func newMetadataHarness(t *testing.T) (*harness, *memorySidecars, *scriptedParser) {
	t.Helper()

	h := newHarnessV2(t)
	sidecars := newMemorySidecars()
	parser := &scriptedParser{results: map[string]scan.NFOResult{
		"heat":   nfoWith(metadata.Record{Title: "Heat NFO"}),
		"heat 2": nfoWith(metadata.Record{Title: "Heat NFO 2"}),
	}}

	refresher, err := scan.NewMetadataRefresher(scan.MetadataRefresherDeps{
		Catalog: h.catalogV2, Reader: sidecars, Parser: parser, Logger: discardLogger(),
	})
	if err != nil {
		t.Fatalf("NewMetadataRefresher() error = %v", err)
	}

	h.metadataRefresher = refresher

	return h, sidecars, parser
}

func TestScanRefreshesMetadataAndNFOChangesTriggerARefresh(t *testing.T) {
	t.Parallel()

	h, sidecars, _ := newMetadataHarness(t)
	h.start(t)

	video := fileAt("Heat (1995)/Heat (1995).mkv", 1)
	sidecars.content["Heat (1995)/Heat (1995).nfo"] = "heat"

	h.scanFiles(t, moviesID, moviesRoot, video, nfoFile("Heat (1995)/Heat (1995).nfo", 2))

	if got := h.catalogV2.titleOf(video.RelativePath); got != "Heat NFO" {
		t.Fatalf("title after the first scan = %q, want Heat NFO", got)
	}

	readsAfterFirst := len(sidecars.reads)

	h.scanFiles(t, moviesID, moviesRoot, video, nfoFile("Heat (1995)/Heat (1995).nfo", 2))

	if len(sidecars.reads) != readsAfterFirst {
		t.Errorf("an unchanged NFO was read again (%d reads)", len(sidecars.reads)-readsAfterFirst)
	}

	sidecars.content["Heat (1995)/Heat (1995).nfo"] = "heat 2"
	h.scanFiles(t, moviesID, moviesRoot, video, nfoFile("Heat (1995)/Heat (1995).nfo", 3))

	if got := h.catalogV2.titleOf(video.RelativePath); got != "Heat NFO 2" {
		t.Errorf("title after the NFO changed = %q, want Heat NFO 2", got)
	}
}

func TestScanCountsMetadataWarnings(t *testing.T) {
	t.Parallel()

	h, sidecars, _ := newMetadataHarness(t)
	h.start(t)

	sidecars.content["Heat (1995)/Heat (1995).nfo"] = "not an nfo"

	status := h.scanFiles(t, moviesID, moviesRoot,
		fileAt("Heat (1995)/Heat (1995).mkv", 1), nfoFile("Heat (1995)/Heat (1995).nfo", 2))

	assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)

	if status.Counts.MetadataWarnings != 1 {
		t.Errorf("metadata warnings = %d, want 1", status.Counts.MetadataWarnings)
	}

	if got := h.catalogV2.titleOf("Heat (1995)/Heat (1995).mkv"); got != "Heat" {
		t.Errorf("title = %q, want the file-name title", got)
	}
}

func TestScanCanceledDuringTheMetadataStep(t *testing.T) {
	t.Parallel()

	h, sidecars, _ := newMetadataHarness(t)
	service := h.start(t)

	sidecars.content["Heat (1995)/Heat (1995).nfo"] = "heat"
	sidecars.onRead = func(ctx context.Context) error {
		closeContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), testTimeout)
		go func() {
			defer cancel()

			_ = service.Close(closeContext)
		}()

		<-ctx.Done()

		return ctx.Err()
	}

	status := h.scanFiles(t, moviesID, moviesRoot,
		fileAt("Heat (1995)/Heat (1995).mkv", 1), nfoFile("Heat (1995)/Heat (1995).nfo", 2))

	assertState(t, status, scan.StateFailed, scan.ErrorCodeCanceled)
}

func TestScanPassesPlacedFilesToTheRefresher(t *testing.T) {
	t.Parallel()

	h := newHarnessV2(t)
	h.start(t)

	h.scanFiles(t, moviesID, moviesRoot, fileAt("Alien (1979).mkv", 1), imageFile("Alien (1979)-poster.jpg", 2))

	request := h.refresher.last
	if len(request.Files) != 1 || len(request.Files[0].ItemIDs) != len(request.Files[0].Placement.Levels) ||
		!request.Files[0].PicturesKnown || len(request.Sidecars) != 1 || request.CountMetadataWarnings == nil {
		t.Errorf("request = %+v, want one placed file with probed pictures, the sidecar and a warning counter", request)
	}
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
