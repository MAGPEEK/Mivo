package scan_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

// fakeSubtitleLister answers from a script keyed by folder and records what it was asked.
type fakeSubtitleLister struct {
	mutex   sync.Mutex
	byDir   map[string]map[string][]media.ExternalSubtitle
	failDir map[string]error
	asked   map[string][]string
}

func newFakeSubtitleLister() *fakeSubtitleLister {
	return &fakeSubtitleLister{
		byDir:   map[string]map[string][]media.ExternalSubtitle{},
		failDir: map[string]error{},
		asked:   map[string][]string{},
	}
}

func (lister *fakeSubtitleLister) ListExternalSubtitles(
	_ context.Context, _, folder string, videoBases []string,
) (map[string][]media.ExternalSubtitle, error) {
	lister.mutex.Lock()
	defer lister.mutex.Unlock()

	lister.asked[folder] = videoBases

	if err := lister.failDir[folder]; err != nil {
		return nil, err
	}

	return lister.byDir[folder], nil
}

func (lister *fakeSubtitleLister) set(folder, base string, subtitles ...media.ExternalSubtitle) {
	lister.mutex.Lock()
	defer lister.mutex.Unlock()

	if lister.byDir[folder] == nil {
		lister.byDir[folder] = map[string][]media.ExternalSubtitle{}
	}

	lister.byDir[folder][base] = subtitles
}

func (lister *fakeSubtitleLister) fail(folder string, err error) {
	lister.mutex.Lock()
	defer lister.mutex.Unlock()

	lister.failDir[folder] = err
}

// memorySubtitleCatalog stores subtitles by file ID and counts the writes.
type memorySubtitleCatalog struct {
	mutex   sync.Mutex
	stored  map[string][]media.ExternalSubtitle
	writes  int
	failing error
}

func (catalog *memorySubtitleCatalog) ExternalSubtitles(
	context.Context, string,
) (map[string][]media.ExternalSubtitle, error) {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	if catalog.failing != nil {
		return nil, catalog.failing
	}

	copied := make(map[string][]media.ExternalSubtitle, len(catalog.stored))
	for id, subtitles := range catalog.stored {
		copied[id] = slices.Clone(subtitles)
	}

	return copied, nil
}

func (catalog *memorySubtitleCatalog) ReplaceExternalSubtitles(
	_ context.Context, fileID string, subtitles []media.ExternalSubtitle,
) error {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	catalog.writes++

	if len(subtitles) == 0 {
		delete(catalog.stored, fileID)

		return nil
	}

	catalog.stored[fileID] = slices.Clone(subtitles)

	return nil
}

func (catalog *memorySubtitleCatalog) writeCount() int {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	return catalog.writes
}

func (catalog *memorySubtitleCatalog) subtitlesOf(fileID string) []media.ExternalSubtitle {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	return slices.Clone(catalog.stored[fileID])
}

func subtitleOf(path, language string) media.ExternalSubtitle {
	return media.ExternalSubtitle{
		RelativePath: path, Format: media.SubtitleFormatSubRip, Language: language, TextBased: true,
	}
}

// subtitleHarness is a harness with the subtitle ports wired to fakes.
type subtitleHarness struct {
	*harness
	lister  *fakeSubtitleLister
	catalog *memorySubtitleCatalog
}

func newSubtitleHarness(t *testing.T) subtitleHarness {
	t.Helper()

	h := newHarness(t)
	lister := newFakeSubtitleLister()
	catalog := &memorySubtitleCatalog{stored: map[string][]media.ExternalSubtitle{}}
	h.subtitles = scan.SubtitlePorts{Lister: lister, Catalog: catalog}
	h.start(t)

	return subtitleHarness{harness: h, lister: lister, catalog: catalog}
}

// fileID returns the catalog ID of the file stored at path.
func (h subtitleHarness) fileID(t *testing.T, path string) string {
	t.Helper()

	files, err := h.catalogV2.PlacedFiles(context.Background(), moviesID)
	if err != nil {
		t.Fatal(err)
	}

	for _, file := range files {
		if file.State.RelativePath == path {
			return file.ID
		}
	}

	t.Fatalf("file %q is not in the catalog", path)

	return ""
}

func TestSubtitlesAreRecordedForEachVideo(t *testing.T) {
	t.Parallel()

	h := newSubtitleHarness(t)
	english := subtitleOf("Heat (1995)/Heat (1995).en.srt", "en")
	german := subtitleOf("Heat (1995)/Subs/Heat (1995).de.srt", "de")
	h.lister.set("Heat (1995)", "Heat (1995)", english, german)

	h.scanFiles(t, moviesID, moviesRoot, fileAt("Heat (1995)/Heat (1995).mkv", 1), fileAt("Alien (1979).mkv", 2))

	got := h.catalog.subtitlesOf(h.fileID(t, "Heat (1995)/Heat (1995).mkv"))
	if !slices.Equal(got, []media.ExternalSubtitle{english, german}) {
		t.Errorf("subtitles = %v, want the two listed", got)
	}

	if h.catalog.writeCount() != 1 {
		t.Errorf("writes = %d, want 1 (Alien has no subtitles and nothing stored)", h.catalog.writeCount())
	}

	if want := []string{"Heat (1995)"}; !slices.Equal(h.lister.asked["Heat (1995)"], want) {
		t.Errorf("asked bases = %v, want %v", h.lister.asked["Heat (1995)"], want)
	}

	if _, asked := h.lister.asked[""]; !asked {
		t.Error("the library root folder was not listed")
	}
}

func TestUnchangedSubtitlesCauseNoWrite(t *testing.T) {
	t.Parallel()

	h := newSubtitleHarness(t)
	video := fileAt("Alien (1979).mkv", 1)
	h.lister.set("", "Alien (1979)", subtitleOf("Alien (1979).en.srt", "en"))

	h.scanFiles(t, moviesID, moviesRoot, video)
	h.scanFiles(t, moviesID, moviesRoot, video)

	if got := h.catalog.writeCount(); got != 1 {
		t.Errorf("writes = %d, want 1 for two identical scans", got)
	}
}

func TestChangedAndRemovedSubtitlesAreReplaced(t *testing.T) {
	t.Parallel()

	h := newSubtitleHarness(t)
	video := fileAt("Alien (1979).mkv", 1)
	h.lister.set("", "Alien (1979)", subtitleOf("Alien (1979).en.srt", "en"), subtitleOf("Alien (1979).de.srt", "de"))
	h.scanFiles(t, moviesID, moviesRoot, video)

	h.lister.set("", "Alien (1979)", subtitleOf("Alien (1979).en.srt", "en"))
	h.scanFiles(t, moviesID, moviesRoot, video)

	id := h.fileID(t, video.RelativePath)
	if got := h.catalog.subtitlesOf(id); len(got) != 1 || got[0].Language != "en" {
		t.Errorf("after a removed file subtitles = %v, want only english", got)
	}

	h.lister.set("", "Alien (1979)")
	h.scanFiles(t, moviesID, moviesRoot, video)

	if got := h.catalog.subtitlesOf(id); len(got) != 0 {
		t.Errorf("subtitles = %v, want none once the files are gone", got)
	}
}

func TestSubtitlesAreCappedPerVideo(t *testing.T) {
	t.Parallel()

	h := newSubtitleHarness(t)

	var many []media.ExternalSubtitle
	for position := range 70 {
		many = append(many, subtitleOf(fmt.Sprintf("Alien (1979).part%02d.srt", position), "und"))
	}

	h.lister.set("", "Alien (1979)", many...)
	h.scanFiles(t, moviesID, moviesRoot, fileAt("Alien (1979).mkv", 1))

	if got := len(h.catalog.subtitlesOf(h.fileID(t, "Alien (1979).mkv"))); got != 64 {
		t.Errorf("stored %d subtitles, want 64", got)
	}
}

func TestAFolderThatCannotBeListedKeepsItsSubtitles(t *testing.T) {
	t.Parallel()

	h := newSubtitleHarness(t)
	video := fileAt("Alien (1979).mkv", 1)
	h.lister.set("", "Alien (1979)", subtitleOf("Alien (1979).en.srt", "en"))
	h.scanFiles(t, moviesID, moviesRoot, video)

	h.lister.fail("", fmt.Errorf("list: %w", domain.ErrUnavailable))
	status := h.scanFiles(t, moviesID, moviesRoot, video)

	assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)

	if got := h.catalog.subtitlesOf(h.fileID(t, video.RelativePath)); len(got) != 1 {
		t.Errorf("subtitles = %v, want the stored one kept", got)
	}
}

func TestSubtitleCatalogFailureFailsTheScan(t *testing.T) {
	t.Parallel()

	h := newSubtitleHarness(t)
	h.catalog.failing = errors.New("disk gone")

	status := h.scanFiles(t, moviesID, moviesRoot, fileAt("Alien (1979).mkv", 1))

	assertState(t, status, scan.StateFailed, scan.ErrorCodeCatalogFailed)
}

func TestSubtitleBasesUseTheNormalisedVideoName(t *testing.T) {
	t.Parallel()

	h := newSubtitleHarness(t)
	video := fileAt("Amélie (2001).mkv", 1)
	video.NormalizedPath = "Amélie (2001).mkv"

	h.scanFiles(t, moviesID, moviesRoot, video)

	if want := []string{"Amélie (2001)"}; !slices.Equal(h.lister.asked[""], want) {
		t.Errorf("asked bases = %v, want %v", h.lister.asked[""], want)
	}
}

func TestScanWithoutSubtitlePortsSkipsTheStep(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.start(t)

	status := h.scanFiles(t, moviesID, moviesRoot, fileAt("Alien (1979).mkv", 1))

	assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)
}
