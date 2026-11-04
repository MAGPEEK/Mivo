package scan_test

import (
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

// inFolder puts a file and a sidecar state into a library folder.
func inFolder(rootID string, file scan.RefreshFile) scan.RefreshFile {
	file.State.RootID = rootID

	return file
}

func sidecarIn(rootID string, state library.FileState) library.FileState {
	state.RootID = rootID

	return state
}

func TestReadNFOOffSkipsTheNFOSource(t *testing.T) {
	t.Parallel()

	for _, readNFO := range []bool{true, false} {
		rig := newRefreshRig(t, "m1")
		rig.library.Options.ReadNFO = readNFO
		rig.sidecars.content["Heat (1995)/movie.nfo"] = "heat"
		rig.parser.results["heat"] = nfoWith(metadata.Record{Title: "From NFO"})

		rig.mustRefresh(t, scan.RefreshRequest{
			ItemIDs:  []string{"m1"},
			Files:    []scan.RefreshFile{refreshFile("Heat (1995)/Heat.mkv", "m1", movieLevel("Heat", 1995))},
			Sidecars: []library.FileState{nfoFile("Heat (1995)/movie.nfo", 2)},
		})

		want := "Heat"
		if readNFO {
			want = "From NFO"
		}

		if got := rig.catalog.state("m1").Record.Title; got != want {
			t.Errorf("ReadNFO=%v: title = %q, want %q", readNFO, got, want)
		}

		if !readNFO && len(rig.sidecars.reads) != 0 {
			t.Errorf("ReadNFO off still read %v", rig.sidecars.reads)
		}
	}
}

func TestLocalArtworkOffKeepsOnlyEmbeddedPictures(t *testing.T) {
	t.Parallel()

	for _, localArtwork := range []bool{true, false} {
		rig := newRefreshRig(t, "m1")
		rig.library.Options.LocalArtwork = localArtwork

		file := refreshFile("Heat/Heat.mkv", "m1", movieLevel("Heat", 1995))
		file.EmbeddedPictures, file.PicturesKnown = []int{3}, true

		rig.mustRefresh(t, scan.RefreshRequest{
			ItemIDs: []string{"m1"}, Files: []scan.RefreshFile{file},
			Sidecars: []library.FileState{imageFile("Heat/poster.jpg", 1)},
		})

		want := artwork.SourceEmbedded
		if localArtwork {
			want = artwork.SourceLocal
		}

		if got := rig.catalog.state("m1").CurrentArtwork[artwork.KindPoster].Source; got != want {
			t.Errorf("LocalArtwork=%v: poster source = %q, want %q", localArtwork, got, want)
		}
	}
}

func TestSidecarsAreReadFromTheFolderOfTheirRootID(t *testing.T) {
	t.Parallel()

	rig := newRefreshRig(t, "m1")
	rig.library = multiFolder(library.KindMovies)
	rig.sidecars.content["Heat (1995)/movie.nfo"] = "heat"
	rig.parser.results["heat"] = nfoWith(metadata.Record{Title: "From NFO"})

	// The movie has a file in both folders; only the second folder has the NFO.
	rig.mustRefresh(t, scan.RefreshRequest{
		ItemIDs: []string{"m1"},
		Files: []scan.RefreshFile{
			inFolder(firstRoot, refreshFile("Heat (1995)/Heat.mkv", "m1", movieLevel("Heat", 1995))),
			inFolder(secondRoot, refreshFile("Heat (1995)/Heat.mkv", "m1", movieLevel("Heat", 1995))),
		},
		Sidecars: []library.FileState{sidecarIn(secondRoot, nfoFile("Heat (1995)/movie.nfo", 2))},
	})

	if got := rig.catalog.state("m1").Record.Title; got != "From NFO" {
		t.Errorf("title = %q, want the NFO of the second folder", got)
	}

	if !slices.Equal(rig.sidecars.roots, []string{secondPath}) {
		t.Errorf("read below %v, want [%s]", rig.sidecars.roots, secondPath)
	}
}

func TestSidecarOfAnotherFolderDoesNotDescribeTheFile(t *testing.T) {
	t.Parallel()

	rig := newRefreshRig(t, "m1")
	rig.library = multiFolder(library.KindMovies)
	rig.sidecars.content["Heat (1995)/movie.nfo"] = "heat"
	rig.parser.results["heat"] = nfoWith(metadata.Record{Title: "From NFO"})

	rig.mustRefresh(t, scan.RefreshRequest{
		ItemIDs:  []string{"m1"},
		Files:    []scan.RefreshFile{inFolder(firstRoot, refreshFile("Heat (1995)/Heat.mkv", "m1", movieLevel("Heat", 1995)))},
		Sidecars: []library.FileState{sidecarIn(secondRoot, nfoFile("Heat (1995)/movie.nfo", 2))},
	})

	if got := rig.catalog.state("m1").Record.Title; got != "Heat" {
		t.Errorf("title = %q, want the file-name title: the NFO lies in another folder", got)
	}
}

func TestLocalArtworkCandidatesCarryTheirFolder(t *testing.T) {
	t.Parallel()

	rig := newRefreshRig(t, "m1")
	rig.library = multiFolder(library.KindMovies)

	file := inFolder(secondRoot, refreshFile("Heat/Heat.mkv", "m1", movieLevel("Heat", 1995)))
	file.EmbeddedPictures, file.PicturesKnown = []int{3}, true

	rig.mustRefresh(t, scan.RefreshRequest{
		ItemIDs: []string{"m1"}, Files: []scan.RefreshFile{file},
		Sidecars: []library.FileState{sidecarIn(secondRoot, imageFile("Heat/poster.jpg", 1))},
	})

	candidates := rig.catalog.savedCandidate["m1"]
	if len(candidates) != 2 {
		t.Fatalf("candidates = %+v, want the local and the embedded poster", candidates)
	}

	for _, candidate := range candidates {
		if candidate.Location.RootID != secondRoot {
			t.Errorf("candidate %+v has root %q, want %q", candidate.Location, candidate.Location.RootID, secondRoot)
		}
	}
}
