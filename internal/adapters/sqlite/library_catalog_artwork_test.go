package sqlite_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

const moviePosterPath = "Movie/poster.jpg"

func artworkIDs(t *testing.T, fixture metadataFixture, itemID string) map[string]string {
	t.Helper()

	rows, err := fixture.inspection.QueryContext(t.Context(),
		`SELECT kind || ':' || source || ':' || ifnull(relative_path, '') || ifnull(data_path, ''), id
		FROM artwork WHERE item_id = ?`, itemID)
	if err != nil {
		t.Fatalf("query artwork: %v", err)
	}

	defer func() { _ = rows.Close() }()

	ids := map[string]string{}

	for rows.Next() {
		var key, id string
		if err := rows.Scan(&key, &id); err != nil {
			t.Fatalf("scan artwork: %v", err)
		}

		ids[key] = id
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("artwork rows: %v", err)
	}

	return ids
}

func TestSaveArtworkKeepsIDsOfUnchangedCandidatesAndReplacesTheRest(t *testing.T) {
	t.Parallel()

	fixture := newMetadataFixture(t)
	poster := localCandidate(artwork.KindPoster, moviePosterPath, 100)
	backdrop := localCandidate(artwork.KindBackdrop, "Movie/fanart.jpg", 200)
	selection := map[artwork.Kind]artwork.Candidate{artwork.KindPoster: poster, artwork.KindBackdrop: backdrop}

	if err := fixture.catalog.SaveArtwork(t.Context(), fixture.movieID, []artwork.Candidate{poster, backdrop}, selection); err != nil {
		t.Fatalf("first SaveArtwork error = %v", err)
	}

	first := artworkIDs(t, fixture, fixture.movieID)

	changedPoster := localCandidate(artwork.KindPoster, moviePosterPath, 150)
	if err := fixture.catalog.SaveArtwork(t.Context(), fixture.movieID, []artwork.Candidate{changedPoster},
		map[artwork.Kind]artwork.Candidate{artwork.KindPoster: changedPoster}); err != nil {
		t.Fatalf("second SaveArtwork error = %v", err)
	}

	second := artworkIDs(t, fixture, fixture.movieID)
	posterKey := "poster:local:" + moviePosterPath

	if len(second) != 1 || second[posterKey] != first[posterKey] {
		t.Errorf("artwork after the second save = %v, first = %v, want only the poster with its old ID", second, first)
	}

	state, err := fixture.catalog.ItemMetadataState(t.Context(), fixture.movieID)
	if err != nil {
		t.Fatalf("ItemMetadataState error = %v", err)
	}

	current := state.CurrentArtwork[artwork.KindPoster]
	if current.Location.RelativePath != moviePosterPath || current.Size != 150 || current.Tag() != changedPoster.Tag() {
		t.Errorf("current poster = %+v, want the changed file", current)
	}

	if _, hasBackdrop := state.CurrentArtwork[artwork.KindBackdrop]; hasBackdrop {
		t.Error("backdrop still selected after its file vanished")
	}

	if tag := countRows(t, fixture.inspection, `SELECT count(*) FROM artwork WHERE tag = ?`, changedPoster.Tag()); tag != 1 {
		t.Errorf("rows with the new tag = %d, want 1", tag)
	}
}

func TestSaveArtworkSelectsOneRowPerKindAndAcceptsNoSelection(t *testing.T) {
	t.Parallel()

	fixture := newMetadataFixture(t)
	first := localCandidate(artwork.KindPoster, "Movie/poster.jpg", 1)
	second := localCandidate(artwork.KindPoster, "Movie/folder.jpg", 2)

	for _, chosen := range []artwork.Candidate{first, second} {
		err := fixture.catalog.SaveArtwork(t.Context(), fixture.movieID, []artwork.Candidate{first, second},
			map[artwork.Kind]artwork.Candidate{artwork.KindPoster: chosen})
		if err != nil {
			t.Fatalf("SaveArtwork error = %v", err)
		}

		state, err := fixture.catalog.ItemMetadataState(t.Context(), fixture.movieID)
		if err != nil || state.CurrentArtwork[artwork.KindPoster].Location != chosen.Location {
			t.Fatalf("selected = %+v, %v, want %+v", state.CurrentArtwork[artwork.KindPoster], err, chosen.Location)
		}
	}

	if err := fixture.catalog.SaveArtwork(t.Context(), fixture.movieID, []artwork.Candidate{first, second}, nil); err != nil {
		t.Fatalf("SaveArtwork without selection error = %v", err)
	}

	if selected := countRows(t, fixture.inspection, `SELECT count(*) FROM artwork WHERE selected = 1`); selected != 0 {
		t.Errorf("selected rows = %d, want 0", selected)
	}
}

func TestSaveArtworkKeepsProviderAndUploadRows(t *testing.T) {
	t.Parallel()

	fixture := newMetadataFixture(t)
	execute(t, fixture.inspection, `INSERT INTO artwork (id, item_id, kind, source, data_path, size_bytes,
		modified_at, tag, selected, created_at) VALUES (?, ?, 'poster', 'upload', 'art/up.jpg', 5, 0, 'tag', 1, 0)`,
		testID('a', 1), fixture.movieID)

	if err := fixture.catalog.SaveArtwork(t.Context(), fixture.movieID, nil, nil); err != nil {
		t.Fatalf("SaveArtwork error = %v", err)
	}

	if rows := countRows(t, fixture.inspection, `SELECT count(*) FROM artwork WHERE source = 'upload'`); rows != 1 {
		t.Errorf("upload rows = %d, want 1 kept", rows)
	}

	upload := artwork.Candidate{Kind: artwork.KindPoster, Source: artwork.SourceUpload, Location: artwork.DataLocation("art/up.jpg")}

	err := fixture.catalog.SaveArtwork(t.Context(), fixture.movieID, nil,
		map[artwork.Kind]artwork.Candidate{artwork.KindPoster: upload})
	if err != nil {
		t.Fatalf("SaveArtwork selecting the upload error = %v", err)
	}

	if selected := countRows(t, fixture.inspection, `SELECT count(*) FROM artwork WHERE selected = 1 AND id = ?`, testID('a', 1)); selected != 1 {
		t.Errorf("upload selected = %d, want 1", selected)
	}
}

func TestSaveArtworkLeavesALockedKindUntouched(t *testing.T) {
	t.Parallel()

	fixture := newMetadataFixture(t)
	old := localCandidate(artwork.KindPoster, "Movie/old.jpg", 1)

	if err := fixture.catalog.SaveArtwork(t.Context(), fixture.movieID, []artwork.Candidate{old},
		map[artwork.Kind]artwork.Candidate{artwork.KindPoster: old}); err != nil {
		t.Fatalf("first SaveArtwork error = %v", err)
	}

	fixture.lock(t, fixture.movieID, metadata.FieldArtworkPoster)

	fresh := localCandidate(artwork.KindPoster, "Movie/new.jpg", 2)
	backdrop := localCandidate(artwork.KindBackdrop, "Movie/fanart.jpg", 3)

	if err := fixture.catalog.SaveArtwork(t.Context(), fixture.movieID, []artwork.Candidate{fresh, backdrop},
		map[artwork.Kind]artwork.Candidate{artwork.KindPoster: fresh, artwork.KindBackdrop: backdrop}); err != nil {
		t.Fatalf("second SaveArtwork error = %v", err)
	}

	state, err := fixture.catalog.ItemMetadataState(t.Context(), fixture.movieID)
	if err != nil {
		t.Fatalf("ItemMetadataState error = %v", err)
	}

	if got := state.CurrentArtwork[artwork.KindPoster].Location.RelativePath; got != "Movie/old.jpg" {
		t.Errorf("locked poster = %q, want the old selection", got)
	}

	if !state.LockedArtwork[artwork.KindPoster] || state.LockedArtwork[artwork.KindBackdrop] {
		t.Errorf("LockedArtwork = %v, want only the poster", state.LockedArtwork)
	}

	if got := state.CurrentArtwork[artwork.KindBackdrop].Location.RelativePath; got != "Movie/fanart.jpg" {
		t.Errorf("backdrop = %q, want the new selection", got)
	}

	if rows := countRows(t, fixture.inspection, `SELECT count(*) FROM artwork WHERE relative_path = 'Movie/new.jpg'`); rows != 0 {
		t.Errorf("rows for the locked kind's new candidate = %d, want 0", rows)
	}
}

func TestSaveArtworkStoresEmbeddedPicturesOfTheItemsLibraryOnly(t *testing.T) {
	t.Parallel()

	fixture := newMetadataFixture(t)
	insertFile(t, fixture.inspection, fileSpec{id: testID('f', 1), itemID: fixture.movieID, libraryID: moviesLibraryID, relativePath: "Movie/Movie.mkv"})

	other := testID('m', 9)
	insertItem(t, fixture.inspection, newItemSpec(other, showsLibraryID, "show", "Show"))
	insertFile(t, fixture.inspection, fileSpec{id: testID('f', 2), itemID: other, libraryID: showsLibraryID, relativePath: "Show/Show.mkv"})

	embedded := embeddedCandidate(artwork.KindPoster, "Movie/Movie.mkv", 3)
	if err := fixture.catalog.SaveArtwork(t.Context(), fixture.movieID, []artwork.Candidate{embedded},
		map[artwork.Kind]artwork.Candidate{artwork.KindPoster: embedded}); err != nil {
		t.Fatalf("SaveArtwork embedded error = %v", err)
	}

	state, err := fixture.catalog.ItemMetadataState(t.Context(), fixture.movieID)
	if err != nil || state.CurrentArtwork[artwork.KindPoster].Location != embedded.Location {
		t.Errorf("selected = %+v, %v, want %+v", state.CurrentArtwork[artwork.KindPoster], err, embedded.Location)
	}

	foreign := embeddedCandidate(artwork.KindPoster, "Show/Show.mkv", 0)
	if err := fixture.catalog.SaveArtwork(t.Context(), fixture.movieID, []artwork.Candidate{foreign}, nil); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("SaveArtwork with a foreign media file error = %v, want ErrInvalid", err)
	}

	if rows := countRows(t, fixture.inspection, `SELECT count(*) FROM artwork WHERE media_file_id = ?`, testID('f', 1)); rows != 1 {
		t.Errorf("rows after the rejected save = %d, want the earlier one kept (rolled back)", rows)
	}
}

func TestSaveArtworkRejectsWhatItCannotStore(t *testing.T) {
	t.Parallel()

	poster := localCandidate(artwork.KindPoster, moviePosterPath, 1)
	tests := []struct {
		name       string
		itemID     string
		candidates []artwork.Candidate
		selection  map[artwork.Kind]artwork.Candidate
		want       error
	}{
		{"unknown item", unknownID, nil, nil, domain.ErrNotFound},
		{"unknown kind", "", []artwork.Candidate{localCandidate("square", moviePosterPath, 1)}, nil, domain.ErrInvalid},
		{"provider candidate", "", []artwork.Candidate{{Kind: artwork.KindPoster, Source: artwork.SourceProvider}}, nil, domain.ErrInvalid},
		{"empty path", "", []artwork.Candidate{localCandidate(artwork.KindPoster, "", 1)}, nil, domain.ErrInvalid},
		{"negative size", "", []artwork.Candidate{localCandidate(artwork.KindPoster, moviePosterPath, -1)}, nil, domain.ErrInvalid},
		{
			"selection that is no candidate", "",
			[]artwork.Candidate{poster},
			map[artwork.Kind]artwork.Candidate{artwork.KindPoster: localCandidate(artwork.KindPoster, "x.jpg", 1)},
			domain.ErrInvalid,
		},
		{
			"selection of an unknown kind", "",
			[]artwork.Candidate{poster},
			map[artwork.Kind]artwork.Candidate{"square": poster},
			domain.ErrInvalid,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := newMetadataFixture(t)
			itemID := test.itemID
			if itemID == "" {
				itemID = fixture.movieID
			}

			if err := fixture.catalog.SaveArtwork(t.Context(), itemID, test.candidates, test.selection); !errors.Is(err, test.want) {
				t.Errorf("SaveArtwork error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestDeletingAnItemRemovesItsArtwork(t *testing.T) {
	t.Parallel()

	fixture := newMetadataFixture(t)
	poster := localCandidate(artwork.KindPoster, moviePosterPath, 1)

	if err := fixture.catalog.SaveArtwork(t.Context(), fixture.movieID, []artwork.Candidate{poster}, nil); err != nil {
		t.Fatalf("SaveArtwork error = %v", err)
	}

	execute(t, fixture.inspection, `DELETE FROM items WHERE id = ?`, fixture.movieID)

	if rows := countRows(t, fixture.inspection, `SELECT count(*) FROM artwork`); rows != 0 {
		t.Errorf("artwork rows after the item was deleted = %d, want 0", rows)
	}
}
