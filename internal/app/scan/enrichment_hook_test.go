package scan_test

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

// recordingEnrichment remembers the items a scan queued for online enrichment.
type recordingEnrichment struct {
	mutex sync.Mutex
	calls [][]string
	err   error
}

func (enrichment *recordingEnrichment) EnqueueScanned(_ context.Context, itemIDs []string) error {
	enrichment.mutex.Lock()
	defer enrichment.mutex.Unlock()

	enrichment.calls = append(enrichment.calls, slices.Clone(itemIDs))

	return enrichment.err
}

func (enrichment *recordingEnrichment) last() []string {
	enrichment.mutex.Lock()
	defer enrichment.mutex.Unlock()

	if len(enrichment.calls) == 0 {
		return nil
	}

	return enrichment.calls[len(enrichment.calls)-1]
}

func (enrichment *recordingEnrichment) count() int {
	enrichment.mutex.Lock()
	defer enrichment.mutex.Unlock()

	return len(enrichment.calls)
}

func TestScanQueuesTheRefreshedItemsForEnrichment(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.enrichment = &recordingEnrichment{}
	h.start(t)

	movie := fileAt("Heat (1995)/Heat (1995).mkv", 1)
	other := fileAt("Alien (1979)/Alien (1979).mkv", 2)
	h.scanFiles(t, moviesID, moviesRoot, movie, other)

	if got := h.enrichment.last(); !slices.Equal(got, h.refresher.last.ItemIDs) || len(got) != 2 {
		t.Fatalf("queued %v, want the refreshed items %v", got, h.refresher.last.ItemIDs)
	}

	h.scanFiles(t, moviesID, moviesRoot, movie, other)

	if h.enrichment.count() != 1 {
		t.Errorf("enqueue calls after an unchanged rescan = %d, want 1 (nothing refreshed, nothing queued)", h.enrichment.count())
	}
}

func TestScanSucceedsWhenQueueingEnrichmentFails(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.enrichment = &recordingEnrichment{err: errors.New("queue full")}
	h.start(t)

	status := h.scanFiles(t, moviesID, moviesRoot, fileAt("Heat (1995).mkv", 1))

	assertState(t, status, scan.StateIdle, "")

	if !strings.Contains(h.logs.String(), "could not queue online metadata") {
		t.Errorf("logs = %q, want the enqueue failure logged", h.logs.String())
	}
}

func TestScanFailureQueuesNoEnrichment(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.enrichment = &recordingEnrichment{}
	h.refresher.err = errors.New("boom")
	h.start(t)

	h.scanFiles(t, moviesID, moviesRoot, fileAt("Heat (1995).mkv", 1))

	if h.enrichment.count() != 0 {
		t.Errorf("enqueue calls = %d, want none after a failed refresh", h.enrichment.count())
	}
}

// memoryLocals keeps the local records the refresher saves.
type memoryLocals struct {
	records map[string]scan.LocalRecords
	err     error
}

func (locals *memoryLocals) SaveLocalRecords(_ context.Context, itemID string, records scan.LocalRecords) error {
	if locals.err != nil {
		return locals.err
	}

	locals.records[itemID] = records

	return nil
}

func newLocalsRig(t *testing.T, locals *memoryLocals, itemIDs ...string) *refreshRig {
	t.Helper()

	rig := newRefreshRig(t, itemIDs...)

	subject, err := scan.NewMetadataRefresher(scan.MetadataRefresherDeps{
		Catalog: rig.catalog, Reader: rig.sidecars, Parser: rig.parser, Locals: locals,
		Logger: slog.New(slog.NewTextHandler(rig.logs, nil)),
	})
	if err != nil {
		t.Fatalf("NewMetadataRefresher() error = %v", err)
	}

	rig.subject = subject

	return rig
}

func TestRefresherKeepsTheLocalRecords(t *testing.T) {
	t.Parallel()

	stored := metadata.Record{Title: "Heat (stored)", Summary: "Earlier NFO"}

	tests := []struct {
		name         string
		nfo          string
		embedded     *metadata.Record
		wantNFO      string
		wantKeep     bool
		wantFilename metadata.Record
	}{
		{name: "nfo used", nfo: "heat", wantNFO: "Heat (NFO)", wantFilename: metadata.Record{Title: "Heat", Year: 1995}},
		{name: "nfo unusable", nfo: "broken", wantNFO: stored.Title, wantKeep: true, wantFilename: metadata.Record{Title: "Heat", Year: 1995}},
		{name: "no nfo", wantFilename: metadata.Record{Title: "Heat", Year: 1995}},
		{
			name: "embedded above the file name", embedded: &metadata.Record{Title: "Heat (tag)", Summary: "Tagged"},
			wantFilename: metadata.Record{Title: "Heat (tag)", Year: 1995, Summary: "Tagged"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			locals := &memoryLocals{records: map[string]scan.LocalRecords{}}
			rig := newLocalsRig(t, locals, "m1")
			rig.catalog.set("m1", scan.MetadataState{Record: stored})
			rig.parser.results["heat"] = nfoWith(metadata.Record{Title: "Heat (NFO)"})

			request := scan.RefreshRequest{
				ItemIDs: []string{"m1"},
				Files:   []scan.RefreshFile{refreshFile("Heat (1995)/Heat.mkv", "m1", movieLevel("Heat", 1995))},
			}

			if test.nfo != "" {
				rig.sidecars.content["Heat (1995)/movie.nfo"] = test.nfo
				request.Sidecars = []library.FileState{nfoFile("Heat (1995)/movie.nfo", 2)}
			}

			if test.embedded != nil {
				request.Embedded = map[string]metadata.Record{"m1": *test.embedded}
			}

			rig.mustRefresh(t, request)

			got := locals.records["m1"]
			if got.NFO.Title != test.wantNFO || got.KeepNFO != test.wantKeep {
				t.Errorf("NFO = %q keep %v, want %q keep %v", got.NFO.Title, got.KeepNFO, test.wantNFO, test.wantKeep)
			}

			if got.Filename.Title != test.wantFilename.Title || got.Filename.Year != test.wantFilename.Year ||
				got.Filename.Summary != test.wantFilename.Summary {
				t.Errorf("Filename = %+v, want %+v", got.Filename, test.wantFilename)
			}
		})
	}
}

func TestRefresherFailsWhenTheLocalRecordsCannotBeSaved(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		err     error
		wantErr bool
	}{
		{name: "store error", err: errors.New("disk full"), wantErr: true},
		{name: "item deleted meanwhile", err: domain.ErrNotFound},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			rig := newLocalsRig(t, &memoryLocals{records: map[string]scan.LocalRecords{}, err: test.err}, "m1")

			err := rig.refresh(context.Background(), scan.RefreshRequest{
				ItemIDs: []string{"m1"},
				Files:   []scan.RefreshFile{refreshFile("Heat (1995).mkv", "m1", movieLevel("Heat", 1995))},
			})
			if (err != nil) != test.wantErr {
				t.Errorf("RefreshItems() error = %v, want error %v", err, test.wantErr)
			}
		})
	}
}
