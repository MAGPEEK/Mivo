package scan_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

// matchKeys returns the sorted match keys of the stored items of kind.
func (catalog *memoryCatalogV2) matchKeys(kind library.ItemKind) []string {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	var keys []string

	for _, item := range catalog.items {
		if item.kind == kind {
			keys = append(keys, item.matchKey)
		}
	}

	slices.Sort(keys)

	return keys
}

// chainOfFile returns the item chain of the file at path.
func (catalog *memoryCatalogV2) chainOfFile(path string) []string {
	itemID := catalog.itemOf(path)

	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	return catalog.chainOf(itemID)
}

// scanMusic scans the music library with the given files and returns the final status.
func (h *harness) scanMusic(t *testing.T, files ...library.FileState) scan.Status {
	t.Helper()

	h.walker.set(musicRoot, walkResult{files: files})

	return h.scan(t, musicID)
}

func assertKeys(t *testing.T, what string, got []string, want ...string) {
	t.Helper()

	if !slices.Equal(got, want) {
		t.Errorf("%s = %q, want %q", what, got, want)
	}
}

func TestMusicScanPlacesATaggedAlbum(t *testing.T) {
	t.Parallel()

	h, tags, details := newMusicHarness(t)
	h.start(t)

	tags.script("x/one.flac", tagOutcome{tags: media.AudioTags{
		AlbumArtist: "Queen", Album: "Jazz", Title: "Mustapha", Track: 1, Disc: 2, Year: 1978,
		MusicBrainzAlbumID: validMusicBrainzID,
	}})
	tags.script("x/two.flac", tagOutcome{tags: media.AudioTags{AlbumArtist: "Queen", Album: "Jazz", Title: "Fat Bottomed Girls", Track: 2, Year: 1978}})
	h.prober.script(absolutePath(musicRoot, "x/one.flac"), probeOutcome{description: audioDescription(1)})
	h.prober.script(absolutePath(musicRoot, "x/two.flac"), probeOutcome{description: audioDescription()})

	status := h.scanMusic(t, audioAt("x/one.flac", 1), audioAt("x/two.flac", 2))

	assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)
	assertCounts(t, status, scan.Counts{Added: 2})
	assertKeys(t, "artists", h.catalogV2.matchKeys(library.ItemKindArtist), "queen")
	assertKeys(t, "albums", h.catalogV2.matchKeys(library.ItemKindAlbum), "queen/jazz/1978")
	assertKeys(t, "tracks", h.catalogV2.matchKeys(library.ItemKindTrack), "d1t2 fat bottomed girls", "d2t1 mustapha")

	saved, found := details.of(h.catalogV2.itemOf("x/one.flac"))
	if !found || saved.disc != 2 || saved.tags.MusicBrainzAlbumID != validMusicBrainzID {
		t.Errorf("track details = %+v (found %t), want disc 2 with the album ID", saved, found)
	}

	files := h.refresher.last.Files
	if len(files) != 2 || len(files[0].ItemIDs) != 3 || !files[0].PicturesKnown {
		t.Fatalf("refresh files = %+v, want two placed tracks with known pictures", files)
	}
}

func TestMusicScanKeepsACompilationOneAlbum(t *testing.T) {
	t.Parallel()

	h, tags, _ := newMusicHarness(t)
	h.start(t)

	tags.script("Hits/01 a.mp3", tagOutcome{tags: media.AudioTags{Artist: "One", Album: "Hits", Title: "A", Compilation: true, Year: 1980}})
	tags.script("Hits/02 b.mp3", tagOutcome{tags: media.AudioTags{Artist: "Two", AlbumArtist: "Various Artists", Album: "Hits", Title: "B", Year: 1991}})

	status := h.scanMusic(t, audioAt("Hits/01 a.mp3", 1), audioAt("Hits/02 b.mp3", 2))

	assertCounts(t, status, scan.Counts{Added: 2})
	assertKeys(t, "artists", h.catalogV2.matchKeys(library.ItemKindArtist), "various artists")
	assertKeys(t, "albums", h.catalogV2.matchKeys(library.ItemKindAlbum), "various artists/hits/")
}

func TestMusicScanFallsBackToFoldersWhenTagsAreUnreadable(t *testing.T) {
	t.Parallel()

	h, tags, details := newMusicHarness(t)
	h.start(t)

	unreadable := tagOutcome{err: errors.New("read audio tags 01 Intro.flac: " + domain.ErrUnsupported.Error())}
	tags.script("Band/Demo (2001)/01 Intro.flac", unreadable)
	tags.script("Band/Demo (2001)/CD 2/03 Outro.flac", unreadable)
	// No video stream is no reason to skip an audio file.
	h.prober.script(absolutePath(musicRoot, "Band/Demo (2001)/01 Intro.flac"), probeOutcome{description: audioDescription()})

	status := h.scanMusic(t, audioAt("Band/Demo (2001)/01 Intro.flac", 1), audioAt("Band/Demo (2001)/CD 2/03 Outro.flac", 2))

	assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)
	assertCounts(t, status, scan.Counts{Added: 2, MetadataWarnings: 2})
	assertKeys(t, "albums", h.catalogV2.matchKeys(library.ItemKindAlbum), "band/demo/2001")
	assertKeys(t, "tracks", h.catalogV2.matchKeys(library.ItemKindTrack), "d1t1 intro", "d2t3 outro")

	if saved, _ := details.of(h.catalogV2.itemOf("Band/Demo (2001)/CD 2/03 Outro.flac")); saved.disc != 2 {
		t.Errorf("disc of the folder-placed track = %d, want 2", saved.disc)
	}

	if logs := h.logs.String(); strings.Count(logs, "scan could not read audio tags") != 2 || !strings.Contains(logs, "level=WARN") {
		t.Errorf("want one warning per unreadable file, logs:\n%s", logs)
	}
}

func TestMusicScanStoresAnUnprobeableFileWithItsProbeError(t *testing.T) {
	t.Parallel()

	h, tags, _ := newMusicHarness(t)
	h.start(t)

	h.prober.script(absolutePath(musicRoot, "A/B/01 S.flac"), probeOutcome{err: domain.ErrUnsupported})

	status := h.scanMusic(t, audioAt("A/B/01 S.flac", 1))

	assertCounts(t, status, scan.Counts{Failed: 1})
	assertKeys(t, "tracks", h.catalogV2.matchKeys(library.ItemKindTrack), "d1t1 s")

	if reads := tags.takeReads(); len(reads) != 0 {
		t.Errorf("tags read for a file that could not be probed: %v", reads)
	}
}

func TestMusicScanDropsMalformedMusicBrainzIDsWithAWarning(t *testing.T) {
	t.Parallel()

	h, tags, details := newMusicHarness(t)
	h.start(t)

	tags.script("A/B/01 S.flac", tagOutcome{tags: media.AudioTags{Title: "S", MusicBrainzTrackID: "garbage"}})

	status := h.scanMusic(t, audioAt("A/B/01 S.flac", 1))

	assertCounts(t, status, scan.Counts{Added: 1, MetadataWarnings: 1})

	if saved, found := details.of(h.catalogV2.itemOf("A/B/01 S.flac")); !found || saved.tags.MusicBrainzTrackID != "" {
		t.Errorf("track details = %+v (found %t), want saved without the malformed ID", saved, found)
	}
}

func TestMusicRescanReadsNothingUnchangedAndRemovesGoneFiles(t *testing.T) {
	t.Parallel()

	h, tags, _ := newMusicHarness(t)
	h.start(t)

	files := []library.FileState{audioAt("A/B/01 a.flac", 1), audioAt("A/B/02 b.flac", 2), audioAt("A/C/01 c.flac", 3)}
	h.scanMusic(t, files...)
	tags.takeReads()

	saves := h.catalogV2.saves
	status := h.scanMusic(t, files...)

	assertCounts(t, status, scan.Counts{Unchanged: 3})

	if reads := tags.takeReads(); len(reads) != 0 || h.catalogV2.saves != saves {
		t.Errorf("unchanged rescan read %v and saved %d files, want nothing", reads, h.catalogV2.saves-saves)
	}

	status = h.scanMusic(t, files[0], files[1])

	assertCounts(t, status, scan.Counts{Unchanged: 2, Removed: 1})
	assertKeys(t, "albums", h.catalogV2.matchKeys(library.ItemKindAlbum), "a/b/")

	// The artist that lost an album is due; one of its remaining tracks names it and its folder.
	assertKeys(t, "reads after removing an album", tags.takeReads(), "A/B/01 a.flac")
}

func TestMusicChangeReadsTheUnchangedTracksOfItsAlbumForTheRefresh(t *testing.T) {
	t.Parallel()

	h, tags, _ := newMusicHarness(t)
	h.start(t)

	h.scanMusic(t, audioAt("A/B/01 a.flac", 1), audioAt("A/B/02 b.flac", 2), audioAt("A/C/01 c.flac", 3))
	tags.takeReads()

	status := h.scanMusic(t, audioAt("A/B/01 a.flac", 1), audioAt("A/B/02 b.flac", 20), audioAt("A/C/01 c.flac", 3))

	assertCounts(t, status, scan.Counts{Updated: 1, Unchanged: 2})
	// The changed track is read for its placement, its album sibling for the album cover; the
	// other album only shares the artist, which the changed track already covers.
	assertKeys(t, "reads", tags.takeReads(), "A/B/01 a.flac", "A/B/02 b.flac")

	var refreshed []string
	for _, file := range h.refresher.last.Files {
		refreshed = append(refreshed, file.State.RelativePath)
	}

	assertKeys(t, "refresh files", refreshed, "A/B/02 b.flac", "A/B/01 a.flac")
}

func TestMusicSidecarChangeReadsOneTrackOfTheArtist(t *testing.T) {
	t.Parallel()

	h, tags, _ := newMusicHarness(t)
	h.start(t)

	tracks := []library.FileState{audioAt("A/B/01 a.flac", 1), audioAt("A/C/01 c.flac", 3)}
	h.scanMusic(t, tracks...)
	tags.takeReads()

	artistImage := fileAt("A/artist.jpg", 9)
	artistImage.Class = library.FileClassImage

	status := h.scanMusic(t, append(slices.Clone(tracks), artistImage)...)

	assertCounts(t, status, scan.Counts{Unchanged: 2})
	// Every item below the artist folder is due; both albums need their tracks.
	assertKeys(t, "reads", tags.takeReads(), "A/B/01 a.flac", "A/C/01 c.flac")
}

func TestMusicRescanRefreshesItemsAFailedRefreshLeftPending(t *testing.T) {
	t.Parallel()

	h, tags, _ := newMusicHarness(t)
	h.start(t)

	files := []library.FileState{audioAt("A/B/01 a.flac", 1), audioAt("A/B/02 b.flac", 2)}
	h.refresher.err = errors.New("refresh failed")
	assertState(t, h.scanMusic(t, files...), scan.StateFailed, scan.ErrorCodeFailed)
	tags.takeReads()

	h.refresher.mutex.Lock()
	h.refresher.err = nil
	h.refresher.mutex.Unlock()

	status := h.scanMusic(t, files...)

	assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)
	assertKeys(t, "reads", tags.takeReads(), "A/B/01 a.flac", "A/B/02 b.flac")

	request := h.refresher.last
	if len(request.Files) != 2 || !slices.Contains(request.ItemIDs, h.catalogV2.chainOfFile("A/B/01 a.flac")[1]) {
		t.Errorf("refresh = %d files, items %v; want both tracks and the pending album", len(request.Files), request.ItemIDs)
	}
}

func TestMusicScanFailsWhenTheTagToolCannotRun(t *testing.T) {
	t.Parallel()

	h, tags, _ := newMusicHarness(t)
	h.start(t)

	tags.script("A/B/01 a.flac", tagOutcome{err: domain.ErrUnavailable})

	status := h.scanMusic(t, audioAt("A/B/01 a.flac", 1))

	assertState(t, status, scan.StateFailed, scan.ErrorCodeProberUnavailable)
}

func TestMusicScanWithoutMusicPortsFails(t *testing.T) {
	t.Parallel()

	h, _, _ := newMusicHarness(t)
	h.music = scan.MusicPorts{}
	h.start(t)

	status := h.scanMusic(t, audioAt("A/B/01 a.flac", 1))

	assertState(t, status, scan.StateFailed, scan.ErrorCodeFailed)
}

func TestMusicScanCancelledWhileProbingRemovesNothing(t *testing.T) {
	t.Parallel()

	h, _, _ := newMusicHarness(t)
	h.start(t)
	h.scanMusic(t, audioAt("A/B/01 a.flac", 1), audioAt("A/B/02 b.flac", 2))

	h.prober.entered = make(chan struct{}, 1)
	h.prober.gate = make(chan struct{})
	h.walker.set(musicRoot, walkResult{files: []library.FileState{audioAt("A/B/03 new.flac", 3)}})

	if _, err := h.service.Start(context.Background(), musicID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	receive(t, h.prober.entered, "the probe to start")

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	if err := h.service.Cancel(ctx, musicID); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	assertState(t, h.service.Status(musicID), scan.StateFailed, scan.ErrorCodeCanceled)

	if got := h.catalogV2.storedFiles(); len(got) != 2 {
		t.Errorf("stored files = %v, want both kept: removals follow the reads", got)
	}
}
