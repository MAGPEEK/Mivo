package sqlite_test

import (
	"testing"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/naming"
)

// The music library of the music fixture and its folder.
const (
	musicLibraryID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	musicRoot      = "/media/music"
)

// MusicBrainz IDs of the tagged test album.
const (
	artistMBID = "11111111-1111-4111-8111-111111111111"
	albumMBID  = "22222222-2222-4222-8222-222222222222"
	trackMBID  = "33333333-3333-4333-8333-333333333333"
	otherMBID  = "44444444-4444-4444-8444-444444444444"
)

// musicFixture is a placement fixture with a music library.
type musicFixture struct {
	*placementFixture
	files int
}

func newMusicFixture(t *testing.T) musicFixture {
	t.Helper()

	fixture := newPlacementFixture(t)
	execute(t, fixture.inspection, `INSERT INTO libraries (id, name, kind, created_at, updated_at)
		VALUES (?1, 'Music', 'music', 0, 0)`, musicLibraryID)
	execute(t, fixture.inspection, `INSERT INTO library_roots (id, library_id, path, path_key, position, created_at)
		VALUES (?1, ?1, ?2, ?2, 0, 0)`, musicLibraryID, musicRoot)

	return musicFixture{placementFixture: fixture}
}

// musicTrack is one audio file of a test album.
type musicTrack struct {
	path string
	disc int
	tags media.AudioTags
}

// addTrack places the file with the music rules, saves it and its track details and returns
// the item chain (artist, album, track).
func (fixture *musicFixture) addTrack(t *testing.T, track musicTrack) []string {
	t.Helper()

	placement, err := naming.PlaceMusic(track.path, track.tags)
	if err != nil {
		t.Fatalf("PlaceMusic(%s) error = %v", track.path, err)
	}

	fixture.files++
	record := placedFile(musicLibraryID, testID('f', fixture.files), track.path, placement)
	record.File.Description.Container = media.ContainerOGG
	record.File.Description.FormatName = "ogg"
	record.File.Description.Streams = []media.Stream{audioStream()}

	chain := fixture.save(t, record)

	disc := max(track.disc, 1)
	if err := fixture.catalog.SaveTrackDetails(t.Context(), chain[len(chain)-1], disc, track.tags); err != nil {
		t.Fatalf("SaveTrackDetails(%s) error = %v", track.path, err)
	}

	return chain
}

// providerIDs returns the stored provider IDs of an item as "provider=id".
func (fixture *musicFixture) providerIDs(t *testing.T, itemID string) []string {
	t.Helper()

	return queryStrings(t, fixture.inspection,
		"SELECT provider || '=' || external_id FROM item_provider_ids WHERE item_id = '"+itemID+"' ORDER BY provider")
}

// pageTitles returns the titles of the items of a page.
func pageTitles(page applibrary.ItemPage) []string {
	titles := make([]string, 0, len(page.Items))
	for _, summary := range page.Items {
		titles = append(titles, summary.Item.Title)
	}

	return titles
}

// collectPages reads every page of a listing with the given limit and returns the titles in
// listing order and the number of pages.
func collectPages(
	t *testing.T, limit int, list func(library.PageRequest) (applibrary.ItemPage, error),
) (titles []string, pages int) {
	t.Helper()

	request := library.PageRequest{Limit: limit}

	for pages = 1; ; pages++ {
		page, err := list(request)
		if err != nil {
			t.Fatalf("page %d error = %v", pages, err)
		}

		titles = append(titles, pageTitles(page)...)

		if page.NextCursor == "" {
			return titles, pages
		}

		request.Cursor = page.NextCursor
	}
}
