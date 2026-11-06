package sqlite_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/app/identify"
	"github.com/magpeek/mivo/internal/app/metadatasettings"
	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

// providerDataPath is a data path the artwork fetcher would write.
const providerDataPath = "artwork/0123456789abcdef0123456789abcdef.jpg"

// withMetadataRows adds an episode below season-1, the TMDB ID of show-1 and a music library
// with an artist, an album and a track carrying the MusicBrainz tag IDs.
func withMetadataRows(t *testing.T, fixture enrichmentFixture) {
	t.Helper()

	episode := newItemSpec("episode-1", "lib-2", "episode", "Pilot")
	episode.parentID, episode.number = "season-1", 3
	insertItem(t, fixture.inspection, episode)

	execute(t, fixture.inspection, `INSERT INTO item_provider_ids (item_id, provider, external_id) VALUES ('show-1', 'tmdb', '1399')`)
	execute(t, fixture.inspection, `INSERT INTO libraries (id, name, kind, created_at, updated_at) VALUES ('lib-3', 'Music', 'music', 0, 0)`)

	artist := newItemSpec("artist-1", "lib-3", "artist", "Kraftwerk")
	album := newItemSpec("album-1", "lib-3", "album", "Autobahn")
	album.parentID = "artist-1"
	track := newItemSpec("track-1", "lib-3", "track", "Autobahn")
	track.parentID, track.number = "album-1", 1

	for _, spec := range []itemSpec{artist, album, track} {
		insertItem(t, fixture.inspection, spec)
	}

	execute(t, fixture.inspection, `INSERT INTO item_provider_ids (item_id, provider, external_id)
		VALUES ('track-1', 'musicbrainz_album', 'release-1'), ('track-1', 'musicbrainz_artist', 'artist-mbid')`)
}

func snapshotOf(t *testing.T, fixture enrichmentFixture, itemID string) enrich.ItemSnapshot {
	t.Helper()

	snapshot, err := fixture.store.ItemSnapshot(t.Context(), itemID)
	if err != nil {
		t.Fatalf("ItemSnapshot(%s) error = %v", itemID, err)
	}

	return snapshot
}

func TestItemSnapshotOfAMovie(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	snapshot := snapshotOf(t, fixture, "item-1")

	if snapshot.Kind != library.ItemKindMovie || !snapshot.OnlineEnabled || snapshot.MatchState != match.MatchStateUnmatched {
		t.Errorf("kind %s online %v state %s, want an unmatched movie with online metadata", snapshot.Kind, snapshot.OnlineEnabled, snapshot.MatchState)
	}

	if snapshot.Record.Title != "A" || !snapshot.Locks.Has(metadata.FieldTitle) || len(snapshot.Record.ProviderIDs) != 1 {
		t.Errorf("record %+v locks %v, want the stored record with its title lock", snapshot.Record, snapshot.Locks.Fields())
	}

	if snapshot.Local.Filename.Title != "A" || snapshot.Local.NFO.Title != "" {
		t.Errorf("local = %+v, want the stored record as the file name source before any local row", snapshot.Local)
	}

	poster, selected := snapshot.Artwork.Current[artwork.KindPoster]
	if len(snapshot.Artwork.Candidates) != 1 || !selected || poster.Location.RelativePath != "poster.jpg" {
		t.Errorf("artwork = %+v, want the selected local poster", snapshot.Artwork)
	}
}

func TestItemSnapshotOfAnEpisodeAndAnAlbum(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	withMetadataRows(t, fixture)

	episode := snapshotOf(t, fixture, "episode-1")
	if episode.SeasonNumber != 1 || episode.EpisodeNumber != 3 ||
		!slices.Equal(episode.ParentProviderIDs, []metadata.ProviderID{{Provider: metadata.ProviderTMDB, Value: "1399"}}) {
		t.Errorf("episode = season %d episode %d show IDs %v, want 1, 3 and the show's TMDB ID",
			episode.SeasonNumber, episode.EpisodeNumber, episode.ParentProviderIDs)
	}

	season := snapshotOf(t, fixture, "season-1")
	if season.SeasonNumber != 1 || len(season.ParentProviderIDs) != 1 {
		t.Errorf("season = %d %v, want 1 and the show's IDs", season.SeasonNumber, season.ParentProviderIDs)
	}

	album := snapshotOf(t, fixture, "album-1")
	wantAlbum := match.ProviderRef{Provider: match.RefMusicBrainz, Kind: match.RefKindRelease, ID: "release-1"}

	if album.ArtistName != "Kraftwerk" || album.LinkedRef != wantAlbum {
		t.Errorf("album = artist %q ref %+v, want Kraftwerk and %+v", album.ArtistName, album.LinkedRef, wantAlbum)
	}

	artist := snapshotOf(t, fixture, "artist-1")
	if artist.LinkedRef != (match.ProviderRef{Provider: match.RefMusicBrainz, Kind: match.RefKindArtist, ID: "artist-mbid"}) {
		t.Errorf("artist ref = %+v, want the tag's artist MBID", artist.LinkedRef)
	}
}

func TestItemSnapshotOfAnUnknownItem(t *testing.T) {
	t.Parallel()

	_, err := newEnrichmentFixture(t).store.ItemSnapshot(t.Context(), "item-9")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func providerPoster() artwork.Candidate {
	return artwork.Candidate{
		Kind: artwork.KindPoster, Source: artwork.SourceProvider, Location: artwork.DataLocation(providerDataPath),
		Size: 2048, ModifiedAt: enrichmentStart(),
	}
}

func tmdbEnrichment() enrich.Enrichment {
	poster := providerPoster()

	return enrich.Enrichment{
		Record: metadata.Record{Title: "The Matrix", Year: 1999, Summary: "A hacker learns the truth."},
		Match: enrich.MatchResult{
			State: match.MatchStateMatched, Provider: match.RefTMDB,
			Ref: match.ProviderRef{Provider: match.RefTMDB, Kind: match.RefKindMovie, ID: "603"}, Confidence: 0.9,
		},
		ProviderArtwork: []artwork.Candidate{poster},
		Selection:       map[artwork.Kind]artwork.Candidate{artwork.KindPoster: poster},
	}
}

func TestSaveEnrichmentStoresRecordMatchAndProviderArtwork(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	if err := fixture.store.SaveEnrichment(t.Context(), "item-1", tmdbEnrichment()); err != nil {
		t.Fatalf("SaveEnrichment() error = %v", err)
	}

	snapshot := snapshotOf(t, fixture, "item-1")

	if snapshot.Record.Title != "A" || snapshot.Record.Summary != "A hacker learns the truth." {
		t.Errorf("record = %+v, want the locked title kept and the provider summary", snapshot.Record)
	}

	if snapshot.MatchState != match.MatchStateMatched || snapshot.LinkedRef.ID != "603" {
		t.Errorf("match = %s %+v, want matched by TMDB 603", snapshot.MatchState, snapshot.LinkedRef)
	}

	if current := snapshot.Artwork.Current[artwork.KindPoster]; current.Source != artwork.SourceProvider ||
		current.Location.DataPath != providerDataPath || len(snapshot.Artwork.Candidates) != 2 {
		t.Errorf("artwork = %+v, want the provider poster selected next to the local one", snapshot.Artwork)
	}

	if snapshot.Local.Filename.Title != "A" || snapshot.Local.Filename.Summary != "" {
		t.Errorf("local = %+v, want the record before enrichment kept as the local source", snapshot.Local)
	}

	again := tmdbEnrichment()
	again.ProviderArtwork, again.Selection = nil, nil

	if err := fixture.store.SaveEnrichment(t.Context(), "item-1", again); err != nil {
		t.Fatalf("second SaveEnrichment() error = %v", err)
	}

	if after := snapshotOf(t, fixture, "item-1"); len(after.Artwork.Candidates) != 1 || len(after.Artwork.Current) != 0 {
		t.Errorf("artwork after a save without provider pictures = %+v, want only the local candidate", after.Artwork)
	}
}

func TestSaveEnrichmentRejectsAMalformedPicture(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	enrichment := tmdbEnrichment()
	enrichment.ProviderArtwork[0].Location.DataPath = "../outside.jpg"

	if err := fixture.store.SaveEnrichment(t.Context(), "item-1", enrichment); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("error = %v, want ErrInvalid", err)
	}
}

func TestAnInFlightEnrichmentCannotUndoARemovedMatch(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	removal := identify.MatchChange{
		Record: metadata.Record{Title: "A"},
		Match:  enrich.MatchResult{State: match.MatchStateDisabled},
	}

	if err := fixture.store.SaveMatchChange(t.Context(), "item-1", removal); err != nil {
		t.Fatalf("SaveMatchChange() error = %v", err)
	}

	if err := fixture.store.SaveEnrichment(t.Context(), "item-1", tmdbEnrichment()); err != nil {
		t.Fatalf("SaveEnrichment() error = %v", err)
	}

	if err := fixture.store.SaveMatchState(t.Context(), "item-1", enrich.MatchResult{State: match.MatchStateUnmatched}); err != nil {
		t.Fatalf("SaveMatchState() error = %v", err)
	}

	snapshot := snapshotOf(t, fixture, "item-1")
	if snapshot.MatchState != match.MatchStateDisabled || snapshot.Record.Summary != "" || len(snapshot.Artwork.Candidates) != 1 {
		t.Errorf("after the late saves: state %s summary %q artwork %d, want disabled and untouched",
			snapshot.MatchState, snapshot.Record.Summary, len(snapshot.Artwork.Candidates))
	}
}

func TestSaveMatchState(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	if err := fixture.store.SaveEnrichment(t.Context(), "item-1", tmdbEnrichment()); err != nil {
		t.Fatalf("SaveEnrichment() error = %v", err)
	}

	err := fixture.store.SaveMatchState(t.Context(), "item-1", enrich.MatchResult{State: match.MatchStateMatched})
	if err != nil || snapshotOf(t, fixture, "item-1").LinkedRef.ID != "603" {
		t.Errorf("matched without a provider: error %v, want the stored reference kept", err)
	}

	if err := fixture.store.SaveMatchState(t.Context(), "item-1", enrich.MatchResult{State: match.MatchStateUnmatched}); err != nil {
		t.Fatalf("SaveMatchState(unmatched) error = %v", err)
	}

	if snapshot := snapshotOf(t, fixture, "item-1"); snapshot.MatchState != match.MatchStateUnmatched || !snapshot.LinkedRef.IsZero() {
		t.Errorf("after unmatched: %s %+v, want unmatched without a reference", snapshot.MatchState, snapshot.LinkedRef)
	}

	if err := fixture.store.SaveMatchState(t.Context(), "item-9", enrich.MatchResult{State: match.MatchStateUnmatched}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown item: error = %v, want ErrNotFound", err)
	}
}

func TestSaveMatchChangeReplacesRecordLocksAndMatch(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	change := identify.MatchChange{
		Record: metadata.Record{Title: "Matrix", ProviderIDs: []metadata.ProviderID{{Provider: metadata.ProviderTMDB, Value: "604"}}},
		Locks:  metadata.NewLockSet([]metadata.Field{metadata.FieldProviderIDs}),
		Match: enrich.MatchResult{
			State: match.MatchStateMatched, Provider: match.RefTMDB, Confidence: 1,
			Ref: match.ProviderRef{Provider: match.RefTMDB, Kind: match.RefKindMovie, ID: "604"},
		},
	}

	if err := fixture.store.SaveMatchChange(t.Context(), "item-1", change); err != nil {
		t.Fatalf("SaveMatchChange() error = %v", err)
	}

	snapshot := snapshotOf(t, fixture, "item-1")
	if snapshot.Record.Title != "Matrix" || snapshot.Locks.Has(metadata.FieldTitle) || !snapshot.Locks.Has(metadata.FieldProviderIDs) {
		t.Errorf("record %q locks %v, want the new title and only the provider_ids lock", snapshot.Record.Title, snapshot.Locks.Fields())
	}

	if id, _ := snapshot.Record.ProviderID(metadata.ProviderTMDB); id != "604" || snapshot.MatchState != match.MatchStateMatched {
		t.Errorf("provider ID %q state %s, want 604 matched", id, snapshot.MatchState)
	}

	if err := fixture.store.SaveMatchChange(t.Context(), "item-9", change); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown item: error = %v, want ErrNotFound", err)
	}
}

func TestSaveLocalRecordsRoundTrip(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	catalog := sqlite.NewLibraryCatalog(fixture.db, time.Now)
	released, err := metadata.ParseDate("1999-03-31")
	if err != nil {
		t.Fatal(err)
	}

	nfo := metadata.Record{Title: "The Matrix", ReleaseDate: released, Genres: []string{"Action"}}
	if err := catalog.SaveLocalRecords(t.Context(), "item-1", scan.LocalRecords{NFO: nfo, Filename: metadata.Record{Title: "Matrix"}}); err != nil {
		t.Fatalf("SaveLocalRecords() error = %v", err)
	}

	keep := scan.LocalRecords{NFO: metadata.Record{Title: "ignored"}, KeepNFO: true, Filename: metadata.Record{Title: "Matrix 2"}}
	if err := catalog.SaveLocalRecords(t.Context(), "item-1", keep); err != nil {
		t.Fatalf("SaveLocalRecords(keep) error = %v", err)
	}

	local := snapshotOf(t, fixture, "item-1").Local
	if local.NFO.Title != "The Matrix" || local.NFO.ReleaseDate != released || local.Filename.Title != "Matrix 2" {
		t.Errorf("local = %+v, want the first NFO with its date and the second file name", local)
	}

	if err := catalog.SaveLocalRecords(t.Context(), "item-9", keep); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown item: error = %v, want ErrNotFound", err)
	}
}

func TestIdentifyItemQueries(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	withMetadataRows(t, fixture)

	children, err := fixture.store.SeasonsAndEpisodes(t.Context(), "show-1")
	if err != nil || !slices.Equal(children, []string{"episode-1", "season-1"}) {
		t.Errorf("SeasonsAndEpisodes = %v, %v, want the season and its episode", children, err)
	}

	if err := fixture.store.SetLibraryOnlineMetadata(t.Context(), "lib-2", false); err != nil {
		t.Fatal(err)
	}

	enabled, err := fixture.store.LibraryOnlineEnabled(t.Context(), "lib-2")
	if err != nil || enabled {
		t.Errorf("LibraryOnlineEnabled(lib-2) = %v, %v, want false", enabled, err)
	}

	if _, err := fixture.store.LibraryOnlineEnabled(t.Context(), "lib-9"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown library: error = %v, want ErrNotFound", err)
	}

	setMatch(t, fixture, "album-1", sqlite.ItemMatch{State: match.MatchStateDisabled})

	items, err := fixture.store.MatchableItems(t.Context(), "lib-3")
	if err != nil || !slices.Equal(items, []string{"artist-1"}) {
		t.Errorf("MatchableItems(lib-3) = %v, %v, want the artist only (album disabled, track not matchable)", items, err)
	}
}

func TestEnqueueItemsFiltersKindsLibrariesAndDisabledMatches(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	withMetadataRows(t, fixture)
	setMatch(t, fixture, "album-1", sqlite.ItemMatch{State: match.MatchStateDisabled})

	if err := fixture.store.SetLibraryOnlineMetadata(t.Context(), "lib-2", false); err != nil {
		t.Fatal(err)
	}

	ids := []string{"item-1", "show-1", "episode-1", "artist-1", "album-1", "track-1", "item-9"}
	kinds := enrich.ProviderKinds(enrich.OnlineSettings{Enabled: true, TMDB: true, MusicBrainz: true})

	queued, err := fixture.store.EnqueueItems(t.Context(), ids, kinds, enrich.ReasonScan)
	if err != nil || queued != 2 {
		t.Fatalf("EnqueueItems = %d, %v, want the movie and the artist", queued, err)
	}

	if length, err := fixture.store.QueueLength(t.Context()); err != nil || length != 2 {
		t.Errorf("QueueLength = %d, %v, want 2", length, err)
	}

	if _, err := fixture.store.EnqueueItems(t.Context(), ids, kinds, enrich.Reason("bogus")); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("unknown reason: error = %v, want ErrInvalid", err)
	}
}

func TestRetryFailedJobsMakesBackedOffJobsDue(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	if err := fixture.store.Enqueue(t.Context(), "item-1", enrich.ReasonScan); err != nil {
		t.Fatal(err)
	}

	job, claimed, err := fixture.store.ClaimNextJob(t.Context(), time.Minute)
	if err != nil || !claimed {
		t.Fatalf("ClaimNextJob = %v, %v", claimed, err)
	}

	if err := fixture.store.MarkJobFailed(t.Context(), job, "offline", enrichmentStart().Add(6*time.Hour)); err != nil {
		t.Fatal(err)
	}

	changed, err := fixture.store.RetryFailedJobs(t.Context())
	if err != nil || changed != 1 {
		t.Fatalf("RetryFailedJobs = %d, %v, want 1", changed, err)
	}

	if _, claimed, err := fixture.store.ClaimNextJob(t.Context(), time.Minute); err != nil || !claimed {
		t.Errorf("claim after retry = %v, %v, want the job due now", claimed, err)
	}
}

func TestMetadataSettingsStoreRoundTrip(t *testing.T) {
	t.Parallel()

	fixture := newEnrichmentFixture(t)
	store := sqlite.NewMetadataSettingsStore(fixture.db, nil)

	if _, found, err := store.LoadMetadataSettings(t.Context()); err != nil || found {
		t.Fatalf("LoadMetadataSettings before a save = %v, %v, want nothing stored", found, err)
	}

	want := metadatasettings.State{Online: true, MusicBrainz: true, Language: "de", OwnTMDBKey: "own-key"}
	if err := store.SaveMetadataSettings(t.Context(), want); err != nil {
		t.Fatalf("SaveMetadataSettings() error = %v", err)
	}

	got, found, err := store.LoadMetadataSettings(t.Context())
	if err != nil || !found || got != want {
		t.Errorf("LoadMetadataSettings = %+v, %v, %v, want %+v", got, found, err, want)
	}
}
