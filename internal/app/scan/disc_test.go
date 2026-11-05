package scan_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

// fakeDirectories resolves disc folders like prefixResolver resolves files, or fails for the
// relative paths listed in errs.
type fakeDirectories struct {
	errs map[string]error
}

func (directories fakeDirectories) ResolveDirectory(root, relativePath string) (string, error) {
	if err := directories.errs[relativePath]; err != nil {
		return "", err
	}

	return absolutePath(root, relativePath), nil
}

// fakeTitles returns a one-clip title per disc, or the error scripted for its absolute path,
// and records the calls.
type fakeTitles struct {
	mutex    sync.Mutex
	errs     map[string]error
	resolved []string
}

func (titles *fakeTitles) Resolve(_ context.Context, discPath string, layout media.Layout) (media.DiscTitle, error) {
	titles.mutex.Lock()
	defer titles.mutex.Unlock()

	titles.resolved = append(titles.resolved, string(layout)+":"+discPath)

	if err := titles.errs[discPath]; err != nil {
		return nil, err
	}

	return discTitleOf(layout), nil
}

func discTitleOf(layout media.Layout) media.DiscTitle {
	if layout == media.LayoutISO {
		return media.DiscTitle{{Offset: 2048, Length: 4096, Duration: time.Hour}}
	}

	return media.DiscTitle{{Path: "BDMV/STREAM/00001.m2ts", Duration: time.Hour}}
}

// fakeDiscProber describes every title as an MPEG-2 video with AC-3 audio, or fails with the
// error scripted for its disc root, and records the sources it probed.
type fakeDiscProber struct {
	mutex   sync.Mutex
	errs    map[string]error
	sources []media.DiscSource
	roots   []string
}

func (prober *fakeDiscProber) ProbeDiscTitle(_ context.Context, libraryRoot string, disc media.DiscSource) (media.Description, error) {
	prober.mutex.Lock()
	defer prober.mutex.Unlock()

	prober.sources = append(prober.sources, disc)
	prober.roots = append(prober.roots, libraryRoot)

	if err := prober.errs[disc.RelativePath]; err != nil {
		return media.Description{}, err
	}

	return discDescription(), nil
}

func discDescription() media.Description {
	return media.Description{
		Container: media.ContainerVOB, FormatName: "mpeg", Duration: time.Hour,
		Streams: []media.Stream{
			{Index: 0, Kind: media.StreamKindVideo, Codec: "mpeg2video", Width: 720, Height: 576},
			{Index: 1, Kind: media.StreamKindAudio, Codec: "ac3", Channels: 6},
			{Index: 2, Kind: media.StreamKindSubtitle, Codec: "dvd_subtitle"},
		},
	}
}

// discHarness is a harness with complete disc ports.
func discHarness(t *testing.T) (*harness, *fakeTitles, *fakeDiscProber) {
	t.Helper()

	h := newHarness(t)
	titles := &fakeTitles{errs: map[string]error{}}
	prober := &fakeDiscProber{errs: map[string]error{}}
	h.discs = scan.DiscPorts{Directories: fakeDirectories{errs: map[string]error{}}, Titles: titles, Prober: prober}

	return h, titles, prober
}

func discAt(relativePath string, size int64, layout media.Layout) library.FileState {
	state := fileAt(relativePath, size)
	state.Class = library.FileClassVideo
	state.Layout = layout

	return state
}

// assertSavedDisc checks the layout and probe error of the media file stored at
// want.RelativePath and returns it.
func assertSavedDisc(t *testing.T, catalog *memoryCatalogV2, want media.File) media.File {
	t.Helper()

	file, found := catalog.savedFile(want.RelativePath)
	if !found {
		t.Fatalf("no media file saved at %q", want.RelativePath)
	}

	if file.Layout != want.Layout || file.ProbeError != want.ProbeError {
		t.Errorf("file %q: layout %q, probe error %q, want %q, %q",
			want.RelativePath, file.Layout, file.ProbeError, want.Layout, want.ProbeError)
	}

	return file
}

func savedDisc(relativePath string, layout media.Layout, probeError media.ProbeError) media.File {
	return media.File{RelativePath: relativePath, Layout: layout, ProbeError: probeError}
}

func TestDiscsBecomeMoviesAndEpisodes(t *testing.T) {
	t.Parallel()

	h, titles, prober := discHarness(t)
	h.start(t)

	movies := h.scanFiles(t, moviesID, moviesRoot,
		discAt("Movie (2019)/BDMV", 1, media.LayoutBDMV),
		discAt("Heat (1995)/VIDEO_TS", 2, media.LayoutVideoTS),
	)
	assertCounts(t, movies, scan.Counts{Added: 2})

	shows := h.scanFiles(t, showsID, showsRoot, discAt("Show/Season 1/Show S01E02.iso", 3, media.LayoutISO))
	assertCounts(t, shows, scan.Counts{Added: 1})

	if movies, episodes := h.catalogV2.count(library.ItemKindMovie), h.catalogV2.count(library.ItemKindEpisode); movies != 2 || episodes != 1 {
		t.Errorf("movies, episodes = %d, %d, want 2, 1", movies, episodes)
	}

	file := assertSavedDisc(t, h.catalogV2, savedDisc("Movie (2019)/BDMV", media.LayoutBDMV, media.ProbeErrorNone))
	if file.Description.Duration != time.Hour || len(file.Description.Streams) != len(discDescription().Streams) {
		t.Errorf("description = %+v, want the main title's", file.Description)
	}

	assertSavedDisc(t, h.catalogV2, savedDisc("Show/Season 1/Show S01E02.iso", media.LayoutISO, media.ProbeErrorNone))

	if probed := h.prober.probedPaths(); len(probed) != 0 {
		t.Errorf("file prober probed %v, want no disc probed as a file", probed)
	}

	wantResolved := []string{
		"bdmv:" + absolutePath(moviesRoot, "Movie (2019)"),
		"iso:" + absolutePath(showsRoot, "Show/Season 1/Show S01E02.iso"),
		"video_ts:" + absolutePath(moviesRoot, "Heat (1995)"),
	}
	if got := sortedCopy(titles.resolved); !slices.Equal(got, wantResolved) {
		t.Errorf("resolved %v, want %v", got, wantResolved)
	}

	assertProbedSource(t, prober, media.DiscSource{
		RelativePath: "Movie (2019)", Layout: media.LayoutBDMV, Title: discTitleOf(media.LayoutBDMV),
	}, moviesRoot)
	assertProbedSource(t, prober, media.DiscSource{
		RelativePath: "Show/Season 1/Show S01E02.iso", Layout: media.LayoutISO, Title: discTitleOf(media.LayoutISO),
	}, showsRoot)
}

func assertProbedSource(t *testing.T, prober *fakeDiscProber, want media.DiscSource, root string) {
	t.Helper()

	prober.mutex.Lock()
	defer prober.mutex.Unlock()

	for index, source := range prober.sources {
		if source.RelativePath == want.RelativePath {
			if source.Layout != want.Layout || !slices.Equal(source.Title, want.Title) || prober.roots[index] != root {
				t.Errorf("probed %+v in %q, want %+v in %q", source, prober.roots[index], want, root)
			}

			return
		}
	}

	t.Errorf("disc %q was not probed", want.RelativePath)
}

func sortedCopy(values []string) []string {
	sorted := slices.Clone(values)
	slices.Sort(sorted)

	return sorted
}

// TestDiscFailuresAreStoredWithTheirCode: a disc that cannot be played is still listed, as an
// unavailable file carrying the disc code (item detail errorCode).
func TestDiscFailuresAreStoredWithTheirCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		script   func(titles *fakeTitles, prober *fakeDiscProber, directories fakeDirectories)
		wantCode media.ProbeError
	}{
		{
			name: "encrypted disc",
			script: func(titles *fakeTitles, _ *fakeDiscProber, _ fakeDirectories) {
				titles.errs[absolutePath(moviesRoot, "Movie (2019)")] = fmt.Errorf("resolve: %w", media.ErrDiscEncrypted)
			},
			wantCode: media.ProbeErrorDiscEncrypted,
		},
		{
			name: "disc without a title",
			script: func(titles *fakeTitles, _ *fakeDiscProber, _ fakeDirectories) {
				titles.errs[absolutePath(moviesRoot, "Movie (2019)")] = fmt.Errorf("resolve: %w", media.ErrDiscNoTitle)
			},
			wantCode: media.ProbeErrorDiscNoTitle,
		},
		{
			name: "title ffprobe cannot read",
			script: func(_ *fakeTitles, prober *fakeDiscProber, _ fakeDirectories) {
				prober.errs["Movie (2019)"] = fmt.Errorf("probe: %w", media.ErrDiscUnsupported)
			},
			wantCode: media.ProbeErrorDiscUnsupported,
		},
		{
			name: "unreadable disc structure",
			script: func(titles *fakeTitles, _ *fakeDiscProber, _ fakeDirectories) {
				titles.errs[absolutePath(moviesRoot, "Movie (2019)")] = errors.New("read index.bdmv: I/O error")
			},
			wantCode: media.ProbeErrorFailed,
		},
		{
			name: "disc folder vanished",
			script: func(_ *fakeTitles, _ *fakeDiscProber, directories fakeDirectories) {
				directories.errs["Movie (2019)"] = fmt.Errorf("resolve: %w", domain.ErrNotFound)
			},
			wantCode: media.ProbeErrorFailed,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h, titles, prober := discHarness(t)
			directories := fakeDirectories{errs: map[string]error{}}
			h.discs.Directories = directories
			test.script(titles, prober, directories)
			h.start(t)

			status := h.scanFiles(t, moviesID, moviesRoot, discAt("Movie (2019)/BDMV", 1, media.LayoutBDMV))

			assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)
			assertCounts(t, status, scan.Counts{Failed: 1})
			assertSavedDisc(t, h.catalogV2, savedDisc("Movie (2019)/BDMV", media.LayoutBDMV, test.wantCode))

			if h.catalogV2.count(library.ItemKindMovie) != 1 {
				t.Error("the unavailable disc is not listed as a movie")
			}
		})
	}
}

// TestDiscsWaitForTheDiscPorts: without disc ports a disc is neither stored nor a failure, and
// the entries an older scan stored below it survive; once the ports exist, the next scan
// places and probes it.
func TestDiscsWaitForTheDiscPorts(t *testing.T) {
	t.Parallel()

	h, _, _ := discHarness(t)
	ports := h.discs
	h.discs = scan.DiscPorts{}
	h.start(t)

	assertCounts(t, h.scanFiles(t, moviesID, moviesRoot, fileAt("Heat (1995)/VIDEO_TS/VTS_01_1.vob", 1)), scan.Counts{Added: 1})

	discs := []library.FileState{
		discAt("Heat (1995)/VIDEO_TS", 2, media.LayoutVideoTS), discAt("Alien (1979).iso", 3, media.LayoutISO),
	}

	status := h.scanFiles(t, moviesID, moviesRoot, discs...)

	assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)
	assertCounts(t, status, scan.Counts{})

	if _, found := h.catalogV2.savedFile("Alien (1979).iso"); found {
		t.Error("a disc was stored without disc ports")
	}

	h.discs = ports
	h.start(t)

	status = h.scanFiles(t, moviesID, moviesRoot, discs...)

	assertCounts(t, status, scan.Counts{Added: 2, Removed: 1})
	assertSavedDisc(t, h.catalogV2, savedDisc("Alien (1979).iso", media.LayoutISO, media.ProbeErrorNone))
	assertSavedDisc(t, h.catalogV2, savedDisc("Heat (1995)/VIDEO_TS", media.LayoutVideoTS, media.ProbeErrorNone))
}

// TestUnsupportedDiscIsProbedAgain: disc_unsupported also stands for an FFmpeg build that
// lacks a demuxer or protocol, so an unchanged disc with that code is probed on every scan
// and becomes available once the probe succeeds.
func TestUnsupportedDiscIsProbedAgain(t *testing.T) {
	t.Parallel()

	h, _, prober := discHarness(t)
	prober.errs["Movie (2019)"] = fmt.Errorf("probe: %w", media.ErrDiscUnsupported)
	h.start(t)

	disc := discAt("Movie (2019)/BDMV", 1, media.LayoutBDMV)
	assertCounts(t, h.scanFiles(t, moviesID, moviesRoot, disc), scan.Counts{Failed: 1})
	assertCounts(t, h.scanFiles(t, moviesID, moviesRoot, disc), scan.Counts{Failed: 1})

	prober.mutex.Lock()
	delete(prober.errs, "Movie (2019)")
	prober.mutex.Unlock()

	assertCounts(t, h.scanFiles(t, moviesID, moviesRoot, disc), scan.Counts{Updated: 1})
	assertSavedDisc(t, h.catalogV2, savedDisc("Movie (2019)/BDMV", media.LayoutBDMV, media.ProbeErrorNone))

	assertCounts(t, h.scanFiles(t, moviesID, moviesRoot, disc), scan.Counts{Unchanged: 1})

	if probes := len(prober.sources); probes != 3 {
		t.Errorf("probed %d times, want 3", probes)
	}
}

// TestDiscsAreNoEntriesOfOtherLibraryKinds: the walker reports images and disc folders in
// every library; only movies and shows libraries take them.
func TestDiscsAreNoEntriesOfOtherLibraryKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T) (*harness, string, string)
	}{
		{name: "music", setup: func(t *testing.T) (*harness, string, string) {
			h, _, _ := newMusicHarness(t)

			return h, musicID, musicRoot
		}},
		{name: "audiobooks", setup: func(t *testing.T) (*harness, string, string) {
			h, _, _ := newAudiobookHarness(t)

			return h, booksID, booksRoot
		}},
		{name: "photos", setup: func(t *testing.T) (*harness, string, string) {
			h, _, _ := newPhotoHarness(t)

			return h, photosID, photosRoot
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h, libraryID, root := test.setup(t)
			titles, prober := &fakeTitles{}, &fakeDiscProber{}
			h.discs = scan.DiscPorts{Directories: fakeDirectories{}, Titles: titles, Prober: prober}
			h.start(t)

			status := h.scanFiles(t, libraryID, root,
				discAt("Album/Album.iso", 1, media.LayoutISO), discAt("Concert (2019)/VIDEO_TS", 2, media.LayoutVideoTS))

			assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)

			if status.Counts.Added != 0 || status.Counts.Failed != 0 {
				t.Errorf("counts = %+v, want no disc added or failed", status.Counts)
			}

			if len(titles.resolved) != 0 || len(prober.sources) != 0 || len(h.prober.probedPaths()) != 0 {
				t.Errorf("resolved %v, probed discs %v and files %v, want nothing", titles.resolved, prober.sources, h.prober.probedPaths())
			}
		})
	}
}

// TestDiscReplacesTheLooseFilesOfOlderScans: before discs were detected, a disc's VOBs were
// listed as videos; once the walker reports the disc, they are gone and the disc is the movie.
func TestDiscReplacesTheLooseFilesOfOlderScans(t *testing.T) {
	t.Parallel()

	h, _, _ := discHarness(t)
	h.start(t)

	assertCounts(t, h.scanFiles(t, moviesID, moviesRoot, fileAt("Heat (1995)/VIDEO_TS/VTS_01_1.vob", 1)), scan.Counts{Added: 1})

	status := h.scanFiles(t, moviesID, moviesRoot, discAt("Heat (1995)/VIDEO_TS", 2, media.LayoutVideoTS))

	assertCounts(t, status, scan.Counts{Added: 1, Removed: 1})

	if movies := h.catalogV2.count(library.ItemKindMovie); movies != 1 {
		t.Errorf("movies = %d, want 1", movies)
	}
}

// TestDiscInTheLibraryRootIsUnmatched: a disc folder directly in the root has no name to be
// placed by (naming.PlaceDisc), so it is skipped like an unplaceable file.
func TestDiscInTheLibraryRootIsUnmatched(t *testing.T) {
	t.Parallel()

	h, titles, _ := discHarness(t)
	h.start(t)

	status := h.scanFiles(t, moviesID, moviesRoot, discAt("VIDEO_TS", 1, media.LayoutVideoTS))

	assertCounts(t, status, scan.Counts{Skipped: 1})

	if len(titles.resolved) != 0 {
		t.Errorf("resolved %v, want nothing", titles.resolved)
	}
}

// TestUnchangedDiscIsNotProbedAgain: a disc is change-detected by its index file like a file.
func TestUnchangedDiscIsNotProbedAgain(t *testing.T) {
	t.Parallel()

	h, _, prober := discHarness(t)
	h.start(t)

	disc := discAt("Movie (2019)/BDMV", 1, media.LayoutBDMV)
	h.scanFiles(t, moviesID, moviesRoot, disc)

	status := h.scanFiles(t, moviesID, moviesRoot, disc)

	assertCounts(t, status, scan.Counts{Unchanged: 1})

	if len(prober.sources) != 1 {
		t.Errorf("probed %d times, want 1", len(prober.sources))
	}
}
