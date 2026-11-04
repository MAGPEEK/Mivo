package library_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

func TestMusicKinds(t *testing.T) {
	t.Parallel()

	kind, err := library.ParseKind("music")
	if err != nil || kind != library.KindMusic {
		t.Fatalf("ParseKind(music) = %q, %v", kind, err)
	}

	itemKind, err := library.ItemKindForLibrary(library.KindMusic)
	if err != nil || itemKind != library.ItemKindTrack {
		t.Fatalf("ItemKindForLibrary(music) = %q, %v", itemKind, err)
	}

	for _, text := range []string{"artist", "album", "track"} {
		parsed, err := library.ParseItemKind(text)
		if err != nil || parsed.String() != text {
			t.Errorf("ParseItemKind(%q) = %q, %v", text, parsed, err)
		}
	}

	if _, err := library.ParseItemKind("song"); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("ParseItemKind(song) err = %v, want ErrInvalid", err)
	}
}

func TestMusicHierarchy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		child, parent library.ItemKind
		owns          bool
		want          bool
	}{
		{library.ItemKindArtist, "", false, true},
		{library.ItemKindArtist, library.ItemKindAlbum, false, false},
		{library.ItemKindAlbum, library.ItemKindArtist, false, true},
		{library.ItemKindAlbum, "", false, false},
		{library.ItemKindTrack, library.ItemKindAlbum, true, true},
		{library.ItemKindTrack, library.ItemKindArtist, true, false},
		{library.ItemKindTrack, "", true, false},
	}

	for _, test := range tests {
		if got := library.ValidParent(test.child, test.parent); got != test.want {
			t.Errorf("ValidParent(%q, %q) = %v, want %v", test.child, test.parent, got, test.want)
		}

		if got := test.child.OwnsFiles(); got != test.owns {
			t.Errorf("%q.OwnsFiles() = %v, want %v", test.child, got, test.owns)
		}
	}

	if library.ItemKind("song").OwnsFiles() {
		t.Error("unknown kind must not own files")
	}

	if library.ValidParent("song", "") {
		t.Error("unknown kind must have no valid parent")
	}
}

func TestMusicKeys(t *testing.T) {
	t.Parallel()

	if got := library.ArtistKey("The  Beatles!"); got != "the beatles" {
		t.Errorf("ArtistKey = %q", got)
	}

	if got := library.ArtistKey(" !!! "); got != "!!!" {
		t.Errorf("ArtistKey(!!!) = %q", got)
	}

	if got := library.AlbumKey("AC/DC", "Back In Black", library.NumberOf(1980)); got != "ac dc/back in black/1980" {
		t.Errorf("AlbumKey = %q", got)
	}

	if got := library.AlbumKey("A", "B", library.OptionalNumber{}); got != "a/b/" {
		t.Errorf("AlbumKey without year = %q", got)
	}

	if got := library.TrackKey(2, library.NumberOf(5), "Hey, Jude!"); got != "d2t5 hey jude" {
		t.Errorf("TrackKey = %q", got)
	}

	if got := library.TrackKey(1, library.OptionalNumber{}, "Intro"); got != "d1 intro" {
		t.Errorf("TrackKey without number = %q", got)
	}
}

func TestCompareTracks(t *testing.T) {
	t.Parallel()

	track := func(disc int, number int, name string) library.TrackOrder {
		order := library.TrackOrder{Disc: disc, FileName: name}
		if number > 0 {
			order.Track = library.NumberOf(number)
		}

		return order
	}

	tests := []struct {
		name        string
		left, right library.TrackOrder
		want        int
	}{
		{"disc first", track(1, 9, "z"), track(2, 1, "a"), -1},
		{"disc later", track(2, 1, "a"), track(1, 9, "z"), 1},
		{"track number", track(1, 2, "z"), track(1, 10, "a"), -1},
		{"track number later", track(1, 10, "a"), track(1, 2, "z"), 1},
		{"numbered before unnumbered", track(1, 3, "z"), track(1, 0, "a"), -1},
		{"unnumbered after numbered", track(1, 0, "a"), track(1, 3, "z"), 1},
		{"file name tie break", track(1, 0, "a.flac"), track(1, 0, "B.flac"), -1},
		{"file name later", track(1, 4, "b.flac"), track(1, 4, "A.flac"), 1},
		{"equal", track(1, 4, "a.flac"), track(1, 4, "A.flac"), 0},
	}

	for _, test := range tests {
		if got := library.CompareTracks(test.left, test.right); got != test.want {
			t.Errorf("%s: CompareTracks = %d, want %d", test.name, got, test.want)
		}
	}

	orders := []library.TrackOrder{track(2, 1, "x"), track(1, 2, "b"), track(1, 1, "c"), track(1, 0, "a")}
	slices.SortFunc(orders, library.CompareTracks)

	if orders[0].FileName != "c" || orders[1].FileName != "b" || orders[2].FileName != "a" || orders[3].FileName != "x" {
		t.Errorf("sorted = %v", orders)
	}
}

func TestSortTitleForMusic(t *testing.T) {
	t.Parallel()

	if got := library.SortTitle("The Beatles"); got != "beatles" {
		t.Errorf("SortTitle = %q", got)
	}
}

func TestAudioFileClass(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind   library.Kind
		name   string
		want   library.FileClass
		wantOK bool
	}{
		{library.KindMusic, "a.FLAC", library.FileClassAudio, true},
		{library.KindMusic, "a.m4a", library.FileClassAudio, true},
		{library.KindMusic, "a.alac", library.FileClassAudio, true},
		{library.KindMusic, "a.mp3", library.FileClassAudio, true},
		{library.KindMusic, "a.aac", library.FileClassAudio, true},
		{library.KindMusic, "a.opus", library.FileClassAudio, true},
		{library.KindMusic, "a.ogg", library.FileClassAudio, true},
		{library.KindMusic, "a.wav", library.FileClassAudio, true},
		{library.KindMusic, "a.aiff", library.FileClassAudio, true},
		{library.KindMusic, "a.ape", library.FileClassAudio, true},
		{library.KindMusic, "a.wv", library.FileClassAudio, true},
		{library.KindMusic, "a.dsf", library.FileClassAudio, true},
		{library.KindMusic, "a.dff", library.FileClassAudio, true},
		{library.KindMusic, "cover.jpg", library.FileClassImage, true},
		{library.KindMusic, "album.nfo", library.FileClassNFO, true},
		{library.KindMusic, "a.mkv", "", false},
		{library.KindMusic, "a.txt", "", false},
		{library.KindMovies, "a.flac", "", false},
		{library.KindShows, "a.mp3", "", false},
		{library.KindMovies, "a.mkv", library.FileClassVideo, true},
	}

	for _, test := range tests {
		got, ok := library.ClassifyFileIn(test.kind, test.name)
		if got != test.want || ok != test.wantOK {
			t.Errorf("ClassifyFileIn(%q, %q) = %q, %v, want %q, %v", test.kind, test.name, got, ok, test.want, test.wantOK)
		}
	}
}

func TestAudioFileClassValidity(t *testing.T) {
	t.Parallel()

	parsed, err := library.ParseFileClass("audio")
	if err != nil || parsed != library.FileClassAudio {
		t.Fatalf("ParseFileClass(audio) = %q, %v", parsed, err)
	}

	tests := []struct {
		class library.FileClass
		kind  library.Kind
		want  bool
	}{
		{library.FileClassAudio, library.KindMusic, true},
		{library.FileClassAudio, library.KindMovies, false},
		{library.FileClassAudio, library.KindShows, false},
		{library.FileClassVideo, library.KindMovies, true},
		{library.FileClass("bogus"), library.KindMusic, false},
	}

	for _, test := range tests {
		if got := test.class.AllowedIn(test.kind); got != test.want {
			t.Errorf("%q.AllowedIn(%q) = %v, want %v", test.class, test.kind, got, test.want)
		}
	}
}
