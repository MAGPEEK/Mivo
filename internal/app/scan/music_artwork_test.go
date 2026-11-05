package scan_test

import (
	"testing"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
)

// trackFile is a track of artist "artist" and album "album" at path; pictures, when given,
// are its attached pictures as probed by this scan.
func trackFile(path, trackID string, pictures ...int) scan.RefreshFile {
	return scan.RefreshFile{
		State: inMoviesFolder(path, 1),
		Placement: library.Placement{Levels: []library.Level{
			{Kind: library.ItemKindArtist, MatchKey: "artist", Title: "Artist"},
			{Kind: library.ItemKindAlbum, MatchKey: "artist/album/", Title: "Album"},
			{Kind: library.ItemKindTrack, MatchKey: trackID, Title: trackID},
		}},
		ItemIDs:          []string{"artist", "album", trackID},
		EmbeddedPictures: pictures,
		PicturesKnown:    true,
	}
}

func TestAlbumCoverOrder(t *testing.T) {
	t.Parallel()

	tracks := []scan.RefreshFile{
		trackFile("Artist/Album/CD 1/01 A.flac", "t1"),
		trackFile("Artist/Album/CD 1/02 B.flac", "t2", 4),
		trackFile("Artist/Album/CD 2/01 C.flac", "t3", 2),
	}

	tests := []struct {
		name     string
		sidecars []library.FileState
		want     artwork.SourceKind
		wantPath string
	}{
		{name: "first track with a picture", want: artwork.SourceEmbedded, wantPath: "Artist/Album/CD 1/02 B.flac"},
		{
			name:     "front image beats embedded",
			sidecars: []library.FileState{imageFile("Artist/Album/front.jpg", 1)},
			want:     artwork.SourceLocal, wantPath: "Artist/Album/front.jpg",
		},
		{
			name: "cover beats folder and front",
			sidecars: []library.FileState{
				imageFile("Artist/Album/front.jpg", 1), imageFile("Artist/Album/folder.jpg", 1),
				imageFile("Artist/Album/cover.png", 1),
			},
			want: artwork.SourceLocal, wantPath: "Artist/Album/cover.png",
		},
		{
			name:     "artist images are no album covers",
			sidecars: []library.FileState{imageFile("Artist/artist.jpg", 1), imageFile("Artist/Album/poster.jpg", 1)},
			want:     artwork.SourceEmbedded, wantPath: "Artist/Album/CD 1/02 B.flac",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			rig := newRefreshRig(t, "artist", "album", "t1", "t2", "t3")
			rig.mustRefresh(t, scan.RefreshRequest{ItemIDs: []string{"album"}, Files: tracks, Sidecars: test.sidecars})

			cover := rig.catalog.state("album").CurrentArtwork[artwork.KindPoster]
			if cover.Source != test.want || cover.Location.RelativePath != test.wantPath {
				t.Errorf("album cover = %+v, want %s at %s", cover, test.want, test.wantPath)
			}

			if _, hasArtwork := rig.catalog.state("t2").CurrentArtwork[artwork.KindPoster]; hasArtwork {
				t.Error("a track got the album cover as its own artwork")
			}
		})
	}
}

func TestArtistImageFromTheArtistFolder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		sidecars []library.FileState
		wantPath string
	}{
		{
			name: "artist.jpg above the album", path: "Artist/Album/01 A.flac",
			sidecars: []library.FileState{imageFile("Artist/artist.jpg", 1), imageFile("Artist/cover.jpg", 1)},
			wantPath: "Artist/artist.jpg",
		},
		{
			name: "album folder in the root has no artist folder", path: "Album/01 A.flac",
			sidecars: []library.FileState{imageFile("artist.jpg", 1)},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			rig := newRefreshRig(t, "artist", "album", "t1")
			rig.mustRefresh(t, scan.RefreshRequest{
				ItemIDs: []string{"artist"}, Files: []scan.RefreshFile{trackFile(test.path, "t1", 0)}, Sidecars: test.sidecars,
			})

			image := rig.catalog.state("artist").CurrentArtwork[artwork.KindPoster]
			if image.Location.RelativePath != test.wantPath {
				t.Errorf("artist image = %+v, want %q", image, test.wantPath)
			}
		})
	}
}

func TestAlbumKeepsTheStoredCoverOfAnUnprobedFirstTrack(t *testing.T) {
	t.Parallel()

	rig := newRefreshRig(t, "artist", "album", "t1", "t2")
	stored := artwork.Candidate{
		Kind: artwork.KindPoster, Source: artwork.SourceEmbedded,
		Location: artwork.EmbeddedLocation("Artist/Album/01 A.flac", 1), Size: 1, ModifiedAt: fileAt("x", 1).ModifiedAt,
	}
	stored.Location.RootID = moviesID
	state := rig.catalog.state("album")
	state.CurrentArtwork = map[artwork.Kind]artwork.Candidate{artwork.KindPoster: stored}
	rig.catalog.set("album", state)

	first := trackFile("Artist/Album/01 A.flac", "t1")
	first.PicturesKnown = false

	rig.mustRefresh(t, scan.RefreshRequest{
		ItemIDs: []string{"album"}, Files: []scan.RefreshFile{first, trackFile("Artist/Album/02 B.flac", "t2", 3)},
	})

	if cover := rig.catalog.state("album").CurrentArtwork[artwork.KindPoster]; cover.Location != stored.Location {
		t.Errorf("album cover = %+v, want the stored one of the first track", cover)
	}
}
