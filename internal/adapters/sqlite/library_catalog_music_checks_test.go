package sqlite_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
)

// The artist ID goes to the album's parent only when that parent is an artist; a corrupted
// hierarchy is reported instead of attaching a MusicBrainz artist ID to another item.
func TestSaveTrackDetailsChecksTheParentOfTheAlbumIsAnArtist(t *testing.T) {
	t.Parallel()

	fixture := newMusicFixture(t)
	track := taggedTrack("Daft Punk/Discovery/01 One More Time.flac", 1, 1, "One More Time")
	chain := fixture.addTrack(t, track)
	other := fixture.addTrack(t, taggedTrack("Daft Punk/Homework/01 Daftendirekt.flac", 1, 1, "Daftendirekt"))

	execute(t, fixture.inspection, "UPDATE items SET parent_id = ?1 WHERE id = ?2", other[1], chain[1])

	err := fixture.catalog.SaveTrackDetails(t.Context(), chain[2], 1, track.tags)
	if !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("SaveTrackDetails below an album whose parent is an album: error = %v, want ErrInvalid", err)
	}
}

func TestMusicListingsFailWithACancelledContext(t *testing.T) {
	t.Parallel()

	fixture := newMusicFixture(t)
	artistID := newDiscography(t, &fixture)
	albumID := albumIDOf(t, fixture)
	ctx := cancelledContext(t)

	listings := map[string]func() error{
		"artist albums": func() error {
			_, err := fixture.reader.ListArtistAlbums(ctx, artistID, library.PageRequest{}, access.Unrestricted())

			return err
		},
		"library albums": func() error {
			_, err := fixture.reader.ListLibraryAlbums(ctx, musicLibraryID, library.PageRequest{}, access.Unrestricted())

			return err
		},
		"album tracks": func() error {
			_, err := fixture.reader.ListAlbumTracks(ctx, albumID, library.PageRequest{}, access.Unrestricted())

			return err
		},
	}

	for name, list := range listings {
		if err := list(); err == nil {
			t.Errorf("%s with a cancelled context succeeded", name)
		}
	}
}
