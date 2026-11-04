package scan_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

// refreshRig wires a MetadataRefresher to fakes.
type refreshRig struct {
	catalog  *metadataCatalog
	sidecars *memorySidecars
	parser   *scriptedParser
	logs     *syncBuffer
	warnings int
	subject  *scan.MetadataRefresher
}

func newRefreshRig(t *testing.T, itemIDs ...string) *refreshRig {
	t.Helper()

	rig := &refreshRig{
		catalog: newMetadataCatalog(itemIDs...), sidecars: newMemorySidecars(),
		parser: &scriptedParser{results: map[string]scan.NFOResult{}}, logs: &syncBuffer{},
	}

	subject, err := scan.NewMetadataRefresher(scan.MetadataRefresherDeps{
		Catalog: rig.catalog, Reader: rig.sidecars, Parser: rig.parser,
		Logger: slog.New(slog.NewTextHandler(rig.logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
	})
	if err != nil {
		t.Fatalf("NewMetadataRefresher() error = %v", err)
	}

	rig.subject = subject

	return rig
}

func (rig *refreshRig) refresh(ctx context.Context, request scan.RefreshRequest) error {
	request.Library = library.Library{ID: moviesID, Root: moviesRoot}
	request.CountMetadataWarnings = func(count int) { rig.warnings += count }

	return rig.subject.RefreshItems(ctx, request)
}

func (rig *refreshRig) mustRefresh(t *testing.T, request scan.RefreshRequest) {
	t.Helper()

	if err := rig.refresh(context.Background(), request); err != nil {
		t.Fatalf("RefreshItems() error = %v", err)
	}
}

func TestNewMetadataRefresherRequiresDependencies(t *testing.T) {
	t.Parallel()

	if _, err := scan.NewMetadataRefresher(scan.MetadataRefresherDeps{}); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("error = %v, want ErrInvalid", err)
	}
}

func TestNFOTitleWinsOverFolderName(t *testing.T) {
	t.Parallel()

	rig := newRefreshRig(t, "m1")
	rig.sidecars.content["Heat (1995)/movie.nfo"] = "heat"
	rig.parser.results["heat"] = nfoWith(metadata.Record{Title: "Heat (Director's Cut)", Genres: []string{"Crime"}})

	rig.mustRefresh(t, scan.RefreshRequest{
		ItemIDs:  []string{"m1"},
		Files:    []scan.RefreshFile{refreshFile("Heat (1995)/Heat.mkv", "m1", movieLevel("Heat", 1995))},
		Sidecars: []library.FileState{nfoFile("Heat (1995)/movie.nfo", 2)},
	})

	record := rig.catalog.state("m1").Record
	if record.Title != "Heat (Director's Cut)" || record.Year != 1995 || len(record.Genres) != 1 {
		t.Errorf("record = %+v, want NFO title and genres with the file-name year", record)
	}

	if len(rig.parser.kinds) != 1 || rig.parser.kinds[0] != scan.NFOKindMovie {
		t.Errorf("parser kinds = %v, want [movie]", rig.parser.kinds)
	}
}

func TestWithoutNFOTheFileNameDataStays(t *testing.T) {
	t.Parallel()

	rig := newRefreshRig(t, "m1")
	rig.mustRefresh(t, scan.RefreshRequest{
		ItemIDs: []string{"m1"},
		Files:   []scan.RefreshFile{refreshFile("Heat (1995).mkv", "m1", movieLevel("Heat", 1995))},
	})

	if record := rig.catalog.state("m1").Record; record.Title != "Heat" || record.Year != 1995 {
		t.Errorf("record = %+v, want the parsed title and year", record)
	}
}

func TestNFORulesPerKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		file     scan.RefreshFile
		itemID   string
		nfoPath  string
		wantKind scan.NFOKind
	}{
		{"loose movie base name", refreshFile("A (2000).mkv", "m", movieLevel("A", 2000)), "m", "A (2000).nfo", scan.NFOKindMovie},
		{"movie.nfo ignored for a loose movie", refreshFile("A (2000).mkv", "m", movieLevel("A", 2000)), "m", "movie.nfo", ""},
		{"show tvshow.nfo", episodeFile("Show/Season 1/e1.mkv", "e1", 1), "show", "Show/tvshow.nfo", scan.NFOKindShow},
		{"season.nfo", episodeFile("Show/Season 1/e1.mkv", "e1", 1), "season", "Show/Season 1/season.nfo", scan.NFOKindSeason},
		{"episode base name", episodeFile("Show/Season 1/e1.mkv", "e1", 1), "e1", "Show/Season 1/e1.nfo", scan.NFOKindEpisode},
		{"case-insensitive name", episodeFile("Show/Season 1/e1.mkv", "e1", 1), "e1", "Show/Season 1/E1.NFO", scan.NFOKindEpisode},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			rig := newRefreshRig(t, test.itemID)
			rig.sidecars.content[test.nfoPath] = "nfo"
			rig.parser.results["nfo"] = nfoWith(metadata.Record{Title: "From NFO"})

			rig.mustRefresh(t, scan.RefreshRequest{
				ItemIDs: []string{test.itemID}, Files: []scan.RefreshFile{test.file},
				Sidecars: []library.FileState{nfoFile(test.nfoPath, 1)},
			})

			wantTitle := "From NFO"
			if test.wantKind == "" {
				wantTitle = "A"
			}

			if got := rig.catalog.state(test.itemID).Record.Title; got != wantTitle {
				t.Errorf("title = %q, want %q", got, wantTitle)
			}
		})
	}
}

func TestMultiEpisodeNFOUsesTheFirstRecord(t *testing.T) {
	t.Parallel()

	rig := newRefreshRig(t, "e1")
	rig.sidecars.content["Show/e1-e2.nfo"] = "multi"
	rig.parser.results["multi"] = scan.NFOResult{Records: []metadata.Record{{Title: "Part one"}, {Title: "Part two"}}}

	file := episodeFile("Show/e1-e2.mkv", "e1", 1)
	rig.mustRefresh(t, scan.RefreshRequest{
		ItemIDs: []string{"e1"}, Files: []scan.RefreshFile{file}, Sidecars: []library.FileState{nfoFile("Show/e1-e2.nfo", 1)},
	})

	if got := rig.catalog.state("e1").Record.Title; got != "Part one" {
		t.Errorf("title = %q, want Part one", got)
	}
}

func TestLockedFieldsKeepStoredValues(t *testing.T) {
	t.Parallel()

	rig := newRefreshRig(t, "m1")
	upload := artwork.Candidate{
		Kind: artwork.KindPoster, Source: artwork.SourceUpload, Location: artwork.DataLocation("artwork/ab/x.jpg"),
		Size: 9, ModifiedAt: fileAt("x", 9).ModifiedAt,
	}
	rig.catalog.set("m1", scan.MetadataState{
		Record:         metadata.Record{Title: "My Title"},
		Locks:          metadata.NewLockSet([]metadata.Field{metadata.FieldTitle}),
		LockedArtwork:  map[artwork.Kind]bool{artwork.KindPoster: true},
		CurrentArtwork: map[artwork.Kind]artwork.Candidate{artwork.KindPoster: upload},
	})
	rig.sidecars.content["Heat/movie.nfo"] = "changed"
	rig.parser.results["changed"] = nfoWith(metadata.Record{Title: "NFO Title", Summary: "New plot"})

	rig.mustRefresh(t, scan.RefreshRequest{
		ItemIDs:  []string{"m1"},
		Files:    []scan.RefreshFile{refreshFile("Heat/Heat.mkv", "m1", movieLevel("Heat", 0))},
		Sidecars: []library.FileState{nfoFile("Heat/movie.nfo", 3), imageFile("Heat/poster.jpg", 4)},
	})

	state := rig.catalog.state("m1")
	if state.Record.Title != "My Title" || state.Record.Summary != "New plot" {
		t.Errorf("record = %+v, want locked title and the new summary", state.Record)
	}

	if state.CurrentArtwork[artwork.KindPoster] != upload || rig.catalog.artworkSaves != 0 {
		t.Errorf("locked poster changed: %+v (saves %d)", state.CurrentArtwork[artwork.KindPoster], rig.catalog.artworkSaves)
	}
}

func TestSeasonPosterFromTheShowFolder(t *testing.T) {
	t.Parallel()

	rig := newRefreshRig(t, "show", "season", "e1")
	rig.mustRefresh(t, scan.RefreshRequest{
		ItemIDs: []string{"show", "season", "e1"},
		Files:   []scan.RefreshFile{episodeFile("Show/Season 1/e1.mkv", "e1", 1)},
		Sidecars: []library.FileState{
			imageFile("Show/poster.jpg", 1), imageFile("Show/fanart.jpg", 2), imageFile("Show/season01-poster.jpg", 3),
			imageFile("Show/season02-poster.jpg", 4), imageFile("Show/Season 1/e1-thumb.jpg", 5),
		},
	})

	wantPaths := map[string]map[artwork.Kind]string{
		"show":   {artwork.KindPoster: "Show/poster.jpg", artwork.KindBackdrop: "Show/fanart.jpg"},
		"season": {artwork.KindPoster: "Show/season01-poster.jpg"},
		"e1":     {artwork.KindThumb: "Show/Season 1/e1-thumb.jpg"},
	}

	for itemID, want := range wantPaths {
		selection := rig.catalog.state(itemID).CurrentArtwork
		if len(selection) != len(want) {
			t.Errorf("%s selection = %+v, want %v", itemID, selection, want)
		}

		for kind, path := range want {
			if got := selection[kind].Location.RelativePath; got != path {
				t.Errorf("%s %s = %q, want %q", itemID, kind, got, path)
			}
		}
	}
}

func TestSeasonFolderPosterBeatsTheShowFolderSeasonPoster(t *testing.T) {
	t.Parallel()

	rig := newRefreshRig(t, "season")
	rig.mustRefresh(t, scan.RefreshRequest{
		ItemIDs: []string{"season"},
		Files:   []scan.RefreshFile{episodeFile("Show/Season 1/e1.mkv", "e1", 1)},
		Sidecars: []library.FileState{
			imageFile("Show/season01-poster.jpg", 3), imageFile("Show/Season 1/poster.jpg", 4),
		},
	})

	if got := rig.catalog.state("season").CurrentArtwork[artwork.KindPoster].Location.RelativePath; got != "Show/Season 1/poster.jpg" {
		t.Errorf("season poster = %q, want the one in the season folder", got)
	}
}

func TestLooseMovieIgnoresAFolderWidePoster(t *testing.T) {
	t.Parallel()

	rig := newRefreshRig(t, "a", "b")
	rig.mustRefresh(t, scan.RefreshRequest{
		ItemIDs: []string{"a", "b"},
		Files: []scan.RefreshFile{
			refreshFile("Movies/A (2000).mkv", "a", movieLevel("A", 2000)),
			refreshFile("Movies/B (2001).mkv", "b", movieLevel("B", 2001)),
		},
		Sidecars: []library.FileState{imageFile("Movies/poster.jpg", 1), imageFile("Movies/A (2000)-poster.jpg", 2)},
	})

	if got := rig.catalog.state("a").CurrentArtwork[artwork.KindPoster].Location.RelativePath; got != "Movies/A (2000)-poster.jpg" {
		t.Errorf("A poster = %q, want its own", got)
	}

	if got := len(rig.catalog.state("b").CurrentArtwork); got != 0 {
		t.Errorf("B has %d artwork, want none: poster.jpg belongs to no loose movie", got)
	}
}

func TestEmbeddedCoverOnlyWithoutLocalPoster(t *testing.T) {
	t.Parallel()

	withCover := refreshFile("Heat/Heat.mkv", "m1", movieLevel("Heat", 0))
	withCover.EmbeddedPictures, withCover.PicturesKnown = []int{3}, true

	tests := []struct {
		name     string
		sidecars []library.FileState
		want     artwork.SourceKind
		wantPath string
	}{
		{"no local poster", nil, artwork.SourceEmbedded, "Heat/Heat.mkv"},
		{"local poster", []library.FileState{imageFile("Heat/poster.jpg", 1)}, artwork.SourceLocal, "Heat/poster.jpg"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			rig := newRefreshRig(t, "m1")
			rig.mustRefresh(t, scan.RefreshRequest{
				ItemIDs: []string{"m1"}, Files: []scan.RefreshFile{withCover}, Sidecars: test.sidecars,
			})

			poster := rig.catalog.state("m1").CurrentArtwork[artwork.KindPoster]
			if poster.Source != test.want || poster.Location.RelativePath != test.wantPath {
				t.Errorf("poster = %+v, want %s at %s", poster, test.want, test.wantPath)
			}
		})
	}
}

func TestUnknownPicturesKeepTheStoredEmbeddedCover(t *testing.T) {
	t.Parallel()

	rig := newRefreshRig(t, "m1")
	cover := artwork.Candidate{
		Kind: artwork.KindPoster, Source: artwork.SourceEmbedded, Location: artwork.EmbeddedLocation("Heat/Heat.mkv", 3),
		Size: 1, ModifiedAt: fileAt("x", 1).ModifiedAt,
	}
	rig.catalog.set("m1", scan.MetadataState{
		Record: metadata.Record{Title: "Heat"}, CurrentArtwork: map[artwork.Kind]artwork.Candidate{artwork.KindPoster: cover},
	})

	rig.mustRefresh(t, scan.RefreshRequest{
		ItemIDs: []string{"m1"}, Files: []scan.RefreshFile{refreshFile("Heat/Heat.mkv", "m1", movieLevel("Heat", 0))},
	})

	if rig.catalog.artworkSaves != 0 || rig.catalog.state("m1").CurrentArtwork[artwork.KindPoster] != cover {
		t.Errorf("the embedded cover of a file that was not probed was dropped (saves %d)", rig.catalog.artworkSaves)
	}
}

func TestNothingIsSavedWhenNothingChanged(t *testing.T) {
	t.Parallel()

	rig := newRefreshRig(t, "m1")
	rig.sidecars.content["Heat/movie.nfo"] = "heat"
	rig.parser.results["heat"] = nfoWith(metadata.Record{Title: "Heat", Genres: []string{"Crime"}})

	request := scan.RefreshRequest{
		ItemIDs:  []string{"m1"},
		Files:    []scan.RefreshFile{refreshFile("Heat/Heat.mkv", "m1", movieLevel("Heat", 0))},
		Sidecars: []library.FileState{nfoFile("Heat/movie.nfo", 1), imageFile("Heat/poster.jpg", 2)},
	}

	rig.mustRefresh(t, request)
	// The catalog derives the sort title, which the merged record leaves empty.
	state := rig.catalog.state("m1")
	state.Record.SortTitle = "heat"
	rig.catalog.set("m1", state)
	rig.mustRefresh(t, request)

	if rig.catalog.metadataSaves != 1 || rig.catalog.artworkSaves != 1 {
		t.Errorf("saves = %d metadata, %d artwork, want 1 each", rig.catalog.metadataSaves, rig.catalog.artworkSaves)
	}
}

func TestUnusableNFOKeepsTheFileNameDataAndIsCounted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		content  string
		readErr  error
		wantCode string
	}{
		{"malformed", "garbage", nil, "nfo_malformed"},
		{"unreadable", "", domain.ErrForbidden, "nfo_unreadable"},
		{"oversized", strings.Repeat("x", 1<<20+1), nil, "nfo_oversized"},
		{"oversized by the reader", "", domain.ErrUnsupported, "nfo_oversized"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			rig := newRefreshRig(t, "m1")
			rig.sidecars.content["Heat/movie.nfo"] = test.content
			rig.sidecars.errs["Heat/movie.nfo"] = test.readErr

			rig.mustRefresh(t, scan.RefreshRequest{
				ItemIDs:  []string{"m1"},
				Files:    []scan.RefreshFile{refreshFile("Heat/Heat.mkv", "m1", movieLevel("Heat", 1995))},
				Sidecars: []library.FileState{nfoFile("Heat/movie.nfo", 1)},
			})

			if record := rig.catalog.state("m1").Record; record.Title != "Heat" || record.Year != 1995 {
				t.Errorf("record = %+v, want the file-name data", record)
			}

			logs := rig.logs.String()
			if rig.warnings != 1 || !strings.Contains(logs, "Heat/movie.nfo") || !strings.Contains(logs, test.wantCode) {
				t.Errorf("warnings = %d, logs = %q, want 1 and path and code %s", rig.warnings, logs, test.wantCode)
			}
		})
	}
}

func TestParserWarningsAreCounted(t *testing.T) {
	t.Parallel()

	rig := newRefreshRig(t, "m1")
	rig.sidecars.content["Heat/movie.nfo"] = "heat"
	rig.parser.results["heat"] = nfoWith(metadata.Record{Title: "Heat"}, "unknown_encoding", "bad_year")

	rig.mustRefresh(t, scan.RefreshRequest{
		ItemIDs:  []string{"m1"},
		Files:    []scan.RefreshFile{refreshFile("Heat/Heat.mkv", "m1", movieLevel("Heat", 0))},
		Sidecars: []library.FileState{nfoFile("Heat/movie.nfo", 1)},
	})

	if rig.warnings != 2 || !strings.Contains(rig.logs.String(), "bad_year") {
		t.Errorf("warnings = %d, logs = %q", rig.warnings, rig.logs.String())
	}
}

func TestItemsWithoutMetadataAreRefreshedEvenWhenNotRequested(t *testing.T) {
	t.Parallel()

	rig := newRefreshRig(t, "empty", "done", "other")
	rig.catalog.set("done", scan.MetadataState{Record: metadata.Record{Title: "Old"}})

	rig.mustRefresh(t, scan.RefreshRequest{
		ItemIDs: []string{"other"},
		Files: []scan.RefreshFile{
			refreshFile("Empty (2000).mkv", "empty", movieLevel("Empty", 2000)),
			refreshFile("Done (2001).mkv", "done", movieLevel("Done", 2001)),
			refreshFile("Other (2002).mkv", "other", movieLevel("Other", 2002)),
		},
	})

	titles := []string{
		rig.catalog.state("empty").Record.Title, rig.catalog.state("done").Record.Title,
		rig.catalog.state("other").Record.Title,
	}
	if titles[0] != "Empty" || titles[1] != "Old" || titles[2] != "Other" {
		t.Errorf("titles = %v, want [Empty Old Other]", titles)
	}
}

func TestGoneItemsAreSkipped(t *testing.T) {
	t.Parallel()

	rig := newRefreshRig(t)
	rig.mustRefresh(t, scan.RefreshRequest{
		ItemIDs: []string{"gone", "unknown"},
		Files:   []scan.RefreshFile{refreshFile("Gone (2000).mkv", "gone", movieLevel("Gone", 2000))},
	})

	if rig.catalog.metadataSaves != 0 {
		t.Errorf("metadata saves = %d, want 0", rig.catalog.metadataSaves)
	}
}

func TestCancellationEndsTheRefresh(t *testing.T) {
	t.Parallel()

	rig := newRefreshRig(t, "m1")
	ctx, cancel := context.WithCancel(context.Background())
	rig.sidecars.content["Heat/movie.nfo"] = "heat"
	rig.sidecars.onRead = func(ctx context.Context) error {
		cancel()

		return ctx.Err()
	}

	err := rig.refresh(ctx, scan.RefreshRequest{
		ItemIDs:  []string{"m1"},
		Files:    []scan.RefreshFile{refreshFile("Heat/Heat.mkv", "m1", movieLevel("Heat", 0))},
		Sidecars: []library.FileState{nfoFile("Heat/movie.nfo", 1)},
	})

	if !errors.Is(err, context.Canceled) || rig.warnings != 0 {
		t.Errorf("error = %v, warnings = %d, want Canceled and no warning", err, rig.warnings)
	}
}
