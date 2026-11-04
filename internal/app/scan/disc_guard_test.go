package scan_test

import (
	"testing"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

// Until M3-16h places discs, a disc entry must neither become an item nor delete the entries
// an older scan stored below it.
func TestDiscEntryIsNotPlacedAndKeepsStoredEntries(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.start(t)

	assertCounts(t, h.scanFiles(t, moviesID, moviesRoot, fileAt("Heat (1995)/VIDEO_TS/VTS_01_1.vob", 1)), scan.Counts{Added: 1})

	disc := fileAt("Heat (1995)/VIDEO_TS", 2)
	disc.Layout = media.LayoutVideoTS
	disc.Class = library.FileClassVideo
	iso := fileAt("Alien (1979).iso", 3)
	iso.Layout = media.LayoutISO
	iso.Class = library.FileClassVideo

	status := h.scanFiles(t, moviesID, moviesRoot, disc, iso)

	assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)

	if status.Counts.Removed != 0 || status.Counts.Added != 0 || status.Counts.Failed != 0 {
		t.Errorf("counts = %+v, want nothing added, removed or failed", status.Counts)
	}
}
