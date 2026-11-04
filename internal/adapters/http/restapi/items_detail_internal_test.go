package restapi

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

const (
	idLibrary = "3f6c1d2a-9b8e-4f70-8a1c-2d3e4f5a6b7c"
	idShow    = "4b5c6d7e-8f9a-4b0c-9d1e-2f3a4b5c6d7e"
	idSeason  = "5c6d7e8f-9a0b-4c1d-8e2f-3a4b5c6d7e8f"
	idPrev    = "6d7e8f9a-0b1c-4d2e-8f3a-4b5c6d7e8f9a"
	idEpisode = "8a9b0c1d-2e3f-4a5b-8c6d-7e8f9a0b1c2d"
	idNext    = "9a0b1c2d-3e4f-4a5b-9c6d-7e8f9a0b1c2d"
	idImage   = "7e6d5c4b-3a2f-4e1d-9c0b-a9f8e7d6c5b4"
	idFile    = "1c2d3e4f-5a6b-4c7d-8e9f-0a1b2c3d4e5f"

	absoluteRoot = `D:\Media\Shows`
)

func mapperItem(id string, kind library.ItemKind) library.Item {
	return library.Item{ID: id, LibraryID: idLibrary, Kind: kind, Title: string(kind) + " title"}
}

func mustMarshal(t *testing.T, value any) string {
	t.Helper()

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	return string(encoded)
}

func TestItemSummaryResponseTable(t *testing.T) {
	t.Parallel()

	poster := &artwork.Ref{ID: idImage, Tag: "tag-1"}
	movie := mapperItem(idEpisode, library.ItemKindMovie)
	movie.Year = library.NumberOf(2008)

	season := mapperItem(idSeason, library.ItemKindSeason)
	season.ParentID = idShow
	season.Number = library.NumberOf(0)

	episode := mapperItem(idEpisode, library.ItemKindEpisode)
	episode.ParentID = idSeason
	episode.Number = library.NumberOf(3)
	episode.NumberEnd = library.NumberOf(4)
	episode.AbsoluteNumber = library.NumberOf(130)

	extra := mapperItem(idEpisode, library.ItemKindExtra)
	extra.ExtraType = library.ExtraTypeBehindTheScenes

	file := media.File{ID: idFile, Description: media.Description{Container: media.ContainerMKV}}

	tests := []struct {
		name     string
		summary  applibrary.ItemSummary
		contains []string
		lacks    []string
	}{
		{
			name:     "movie with poster and file",
			summary:  applibrary.ItemSummary{Item: movie, PrimaryFile: file, Poster: poster},
			contains: []string{`"kind":"movie"`, `"year":2008`, `"poster":{"id":"` + idImage + `","tag":"tag-1"}`, `"media":`},
			lacks:    []string{`"thumb"`, `"parentId"`, `"number"`, `"childCount"`, `"extraType"`},
		},
		{
			name:     "show container",
			summary:  applibrary.ItemSummary{Item: mapperItem(idShow, library.ItemKindShow), ChildCount: 5, LeafCount: 62},
			contains: []string{`"kind":"show"`, `"childCount":5`, `"leafCount":62`},
			lacks:    []string{`"media"`, `"year"`, `"poster"`},
		},
		{
			name:     "season with fallback poster and specials number",
			summary:  applibrary.ItemSummary{Item: season, Poster: poster, ChildCount: 8},
			contains: []string{`"number":0`, `"parentId":"` + idShow + `"`, `"poster":`},
			lacks:    []string{`"media"`, `"leafCount"`},
		},
		{
			name:     "episode with numbers and thumb",
			summary:  applibrary.ItemSummary{Item: episode, PrimaryFile: file, Thumb: poster},
			contains: []string{`"number":3`, `"numberEnd":4`, `"absoluteNumber":130`, `"thumb":`, `"parentId":"` + idSeason + `"`},
			lacks:    []string{`"poster"`},
		},
		{
			name:     "extra with type",
			summary:  applibrary.ItemSummary{Item: extra, PrimaryFile: file},
			contains: []string{`"kind":"extra"`, `"extraType":"behind_the_scenes"`},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			response, err := itemSummaryResponse(test.summary)
			if err != nil {
				t.Fatalf("itemSummaryResponse: %v", err)
			}

			assertFragments(t, mustMarshal(t, response), test.contains, test.lacks)
		})
	}
}

func assertFragments(t *testing.T, body string, contains, lacks []string) {
	t.Helper()

	for _, fragment := range contains {
		if !strings.Contains(body, fragment) {
			t.Errorf("body %s lacks %s", body, fragment)
		}
	}

	for _, fragment := range lacks {
		if strings.Contains(body, fragment) {
			t.Errorf("body %s contains %s", body, fragment)
		}
	}
}

func TestItemSummaryResponseRejectsBadStoredData(t *testing.T) {
	t.Parallel()

	unknownKind := mapperItem(idEpisode, "music")
	badParent := mapperItem(idEpisode, library.ItemKindEpisode)
	badParent.ParentID = "nope"

	tests := []struct {
		name    string
		summary applibrary.ItemSummary
	}{
		{name: "unknown kind", summary: applibrary.ItemSummary{Item: unknownKind}},
		{name: "malformed parent", summary: applibrary.ItemSummary{Item: badParent}},
		{name: "malformed image id", summary: applibrary.ItemSummary{Item: mapperItem(idEpisode, library.ItemKindMovie), Poster: &artwork.Ref{ID: "x", Tag: "t"}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := itemSummaryResponse(test.summary); err == nil {
				t.Error("itemSummaryResponse succeeded, want an error")
			}
		})
	}
}

func TestItemDetailResponseEpisode(t *testing.T) {
	t.Parallel()

	episode := mapperItem(idEpisode, library.ItemKindEpisode)
	episode.Year = library.NumberOf(2010)

	file := media.File{
		ID: idFile, RelativePath: "Shows/Show/S01/ep1.mkv", Size: 10,
		ModifiedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Description: media.Description{Container: media.ContainerMKV},
	}

	detail := applibrary.ItemDetail{
		Item: episode,
		Metadata: metadata.Record{
			OriginalTitle: "Orig", Summary: "Plot", Tagline: "Tag", ContentRating: "FSK 12",
			ReleaseDate: mustDate(t, "2010-03-04"),
			Genres:      []string{"Drama"}, Studios: []string{"AMC"}, Countries: []string{"US"},
			People: []metadata.Credit{
				{Name: "Ann", Role: metadata.RoleActor, Character: "Zed"},
				{Name: "Bob", Role: metadata.RoleDirector},
			},
			ProviderIDs: []metadata.ProviderID{{Provider: metadata.ProviderIMDb, Value: "tt1234567"}},
		},
		Ancestors: []applibrary.ItemRef{
			{ID: idShow, Kind: library.ItemKindShow, Title: "Show"},
			{ID: idSeason, Kind: library.ItemKindSeason, Title: "Season 0", Number: library.NumberOf(0)},
		},
		PreviousID: idPrev, NextID: idNext, PlayItemID: idEpisode,
		Artwork: map[artwork.Kind]artwork.Ref{
			artwork.KindThumb: {ID: idImage, Tag: "t1"}, artwork.KindBackdrop: {ID: idImage, Tag: "t2"},
			artwork.KindLogo: {ID: idImage, Tag: "t3"}, artwork.KindBanner: {ID: idImage, Tag: "t4"},
			artwork.KindPoster: {ID: idImage, Tag: "t5"},
		},
		Extras: []applibrary.ItemSummary{{Item: mapperItem(idNext, library.ItemKindExtra), PrimaryFile: file}},
		ItemFiles: []applibrary.ItemFile{
			{File: file, VersionLabel: "4K", PartNumber: library.NumberOf(2)},
		},
		Files: []media.File{file},
	}

	response, err := itemDetailResponse(detail)
	if err != nil {
		t.Fatalf("itemDetailResponse: %v", err)
	}

	body := mustMarshal(t, response)

	for _, fragment := range []string{
		`"originalTitle":"Orig"`, `"summary":"Plot"`, `"tagline":"Tag"`, `"contentRating":"FSK 12"`,
		`"releaseDate":"2010-03-04"`, `"year":2010`, `"genres":["Drama"]`, `"studios":["AMC"]`,
		`"countries":["US"]`, `"role":"actor"`, `"character":"Zed"`, `"imdb":"tt1234567"`,
		`"previousId":"` + idPrev + `"`, `"nextId":"` + idNext + `"`, `"playItemId":"` + idEpisode + `"`,
		`"number":0`, `"versionLabel":"4K"`, `"partNumber":2`, `"extras":[`, `"tag":"t4"`,
		`"fileName":"ep1.mkv"`,
	} {
		if !strings.Contains(body, fragment) {
			t.Errorf("body %s lacks %s", body, fragment)
		}
	}
}

func TestItemDetailResponseOmitsAbsentValues(t *testing.T) {
	t.Parallel()

	detail := applibrary.ItemDetail{Item: mapperItem(idShow, library.ItemKindShow)}

	response, err := itemDetailResponse(detail)
	if err != nil {
		t.Fatalf("itemDetailResponse: %v", err)
	}

	body := mustMarshal(t, response)

	for _, fragment := range []string{
		`"ancestors"`, `"artwork"`, `"extras"`, `"genres"`, `"people"`, `"providerIds"`, `"releaseDate"`,
		`"summary"`, `"nextId"`, `"previousId"`, `"playItemId"`, `"childCount"`, `"year"`,
	} {
		if strings.Contains(body, fragment) {
			t.Errorf("body %s contains %s", body, fragment)
		}
	}

	if !strings.Contains(body, `"files":[]`) {
		t.Errorf("body %s lacks an empty files array", body)
	}
}

func TestItemDetailResponseFallsBackToMetadataYear(t *testing.T) {
	t.Parallel()

	detail := applibrary.ItemDetail{Item: mapperItem(idEpisode, library.ItemKindMovie), Metadata: metadata.Record{Year: 1999}}

	response, err := itemDetailResponse(detail)
	if err != nil {
		t.Fatalf("itemDetailResponse: %v", err)
	}

	if body := mustMarshal(t, response); !strings.Contains(body, `"year":1999`) {
		t.Errorf("body %s lacks the metadata year", body)
	}
}

func TestItemDetailResponseNeverSendsPaths(t *testing.T) {
	t.Parallel()

	file := media.File{
		ID: idFile, RelativePath: `Season 1\ep1.mkv`, Size: 1,
		Description: media.Description{Container: media.ContainerMKV, FormatName: absoluteRoot},
	}
	item := mapperItem(idEpisode, library.ItemKindEpisode)
	detail := applibrary.ItemDetail{Item: item, Files: []media.File{file}, ItemFiles: []applibrary.ItemFile{{File: file}}}

	response, err := itemDetailResponse(detail)
	if err != nil {
		t.Fatalf("itemDetailResponse: %v", err)
	}

	body := mustMarshal(t, response)
	for _, forbidden := range []string{absoluteRoot, `Season 1`, `\\`} {
		if strings.Contains(body, forbidden) {
			t.Errorf("body %s contains %q", body, forbidden)
		}
	}

	if !strings.Contains(body, `"fileName":"ep1.mkv"`) {
		t.Errorf("body %s lacks the bare file name", body)
	}
}

func TestItemDetailResponseRejectsBadStoredData(t *testing.T) {
	t.Parallel()

	valid := mapperItem(idEpisode, library.ItemKindEpisode)

	tests := []struct {
		name   string
		detail applibrary.ItemDetail
	}{
		{name: "unknown person role", detail: applibrary.ItemDetail{Item: valid, Metadata: metadata.Record{People: []metadata.Credit{{Name: "x", Role: "juggler"}}}}},
		{name: "unknown artwork kind", detail: applibrary.ItemDetail{Item: valid, Artwork: map[artwork.Kind]artwork.Ref{"square": {ID: idImage, Tag: "t"}}}},
		{name: "malformed ancestor", detail: applibrary.ItemDetail{Item: valid, Ancestors: []applibrary.ItemRef{{ID: "x", Kind: library.ItemKindShow}}}},
		{name: "malformed sibling", detail: applibrary.ItemDetail{Item: valid, NextID: "x"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := itemDetailResponse(test.detail); err == nil {
				t.Error("itemDetailResponse succeeded, want an error")
			}
		})
	}
}

func TestExtraTypeResponse(t *testing.T) {
	t.Parallel()

	tests := map[library.ExtraType]string{
		library.ExtraTypeTrailer: "trailer", library.ExtraTypeFeaturette: "featurette",
		library.ExtraTypeBehindTheScenes: "behind_the_scenes", library.ExtraTypeDeletedScene: "deleted_scene",
		library.ExtraTypeInterview: "interview", library.ExtraTypeScene: "scene", library.ExtraTypeShort: "short",
		library.ExtraTypeOther: "other", "something new": "other",
	}

	for input, want := range tests {
		got := extraTypeResponse(input)
		if got == nil || string(*got) != want {
			t.Errorf("extraTypeResponse(%q) = %v, want %q", input, got, want)
		}
	}

	if extraTypeResponse("") != nil {
		t.Error("extraTypeResponse(empty) is not nil")
	}
}

func mustDate(t *testing.T, text string) metadata.Date {
	t.Helper()

	date, err := metadata.ParseDate(text)
	if err != nil {
		t.Fatalf("parse date: %v", err)
	}

	return date
}
