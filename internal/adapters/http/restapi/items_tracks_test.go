package restapi_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

type fakeTrackLister struct {
	tracks []playback.SubtitleTrack
	err    error
	asked  []string
}

func (fake *fakeTrackLister) Tracks(_ context.Context, fileID string) ([]playback.SubtitleTrack, error) {
	fake.asked = append(fake.asked, fileID)

	return fake.tracks, fake.err
}

// englishFile has an English default audio stream, a forced English subtitle stream and a
// German picture subtitle.
func englishFile(t *testing.T) media.File {
	t.Helper()

	file := probedFile(t)
	file.Description.Streams = []media.Stream{
		{Index: 0, Kind: media.StreamKindVideo, Codec: "h264", Width: 1920, Height: 1080, Default: true},
		{Index: 1, Kind: media.StreamKindAudio, Codec: "aac", Language: "eng", Default: true},
		{Index: 2, Kind: media.StreamKindSubtitle, Codec: "subrip", Language: "eng", Forced: true},
		{Index: 3, Kind: media.StreamKindSubtitle, Codec: "hdmv_pgs_subtitle", Language: "ger", Title: "Deutsch"},
	}

	return file
}

func getItemFile(t *testing.T, services restapi.StreamingServices, file media.File) apigen.MediaFile {
	t.Helper()

	libraries := &fakeLibraryService{detail: applibrary.ItemDetail{
		Item: testItem(testItemID, "Big Buck Bunny"), Files: []media.File{file},
	}}
	handler, _ := newTestHandlerWithOptions(t, restapi.Options{Libraries: libraries, Items: libraries, Streaming: services})

	got := serve(t, handler, http.MethodGet, itemPath)
	if got.status != http.StatusOK {
		t.Fatalf("status = %d (body %s)", got.status, got.body)
	}

	detail := decodeStrictly[apigen.ItemDetail](t, got.body)
	if len(detail.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(detail.Files))
	}

	return detail.Files[0]
}

func TestMediaFileListsEmbeddedTracksAndDefaults(t *testing.T) {
	t.Parallel()

	file := getItemFile(t, restapi.StreamingServices{}, englishFile(t))

	if file.SubtitleTracks == nil || len(*file.SubtitleTracks) != 2 {
		t.Fatalf("subtitleTracks = %v, want 2", file.SubtitleTracks)
	}

	forced, picture := (*file.SubtitleTracks)[0], (*file.SubtitleTracks)[1]
	if forced.ID != "s:2" || !forced.Forced || !forced.TextBased || forced.External ||
		forced.URL == nil || *forced.URL != "/api/v1/media-files/"+testCatalogFileID+"/subtitles/s:2.vtt" {
		t.Errorf("forced = %+v", forced)
	}

	if picture.ID != "s:3" || picture.TextBased || picture.URL != nil || picture.Format != "hdmv_pgs_subtitle" {
		t.Errorf("picture = %+v", picture)
	}

	if file.DefaultAudioStreamIndex == nil || *file.DefaultAudioStreamIndex != 1 {
		t.Errorf("defaultAudioStreamIndex = %v, want 1", file.DefaultAudioStreamIndex)
	}

	if file.DefaultSubtitleTrackID == nil || *file.DefaultSubtitleTrackID != "s:2" {
		t.Errorf("defaultSubtitleTrackId = %v, want the forced track s:2", file.DefaultSubtitleTrackID)
	}

	if file.Width == nil || *file.Width != 1920 || file.Height == nil || *file.Height != 1080 ||
		file.Resolution == nil || *file.Resolution != apigen.VideoResolutionN1080P {
		t.Errorf("width/height/resolution = %v/%v/%v", file.Width, file.Height, file.Resolution)
	}
}

func TestMediaFileAddsExternalTracksAfterTheEmbeddedOnes(t *testing.T) {
	t.Parallel()

	lister := &fakeTrackLister{tracks: []playback.SubtitleTrack{
		{ID: "x:0123456789ab", Language: "ger", Format: "subrip", External: true, TextBased: true, SDH: true},
	}}

	file := getItemFile(t, restapi.StreamingServices{Tracks: lister}, englishFile(t))

	if len(lister.asked) != 1 || lister.asked[0] != testCatalogFileID {
		t.Errorf("asked = %v, want [%s]", lister.asked, testCatalogFileID)
	}

	tracks := *file.SubtitleTracks
	if len(tracks) != 3 || tracks[2].ID != "x:0123456789ab" || !tracks[2].External || !tracks[2].Sdh ||
		tracks[2].URL == nil || *tracks[2].URL != "/api/v1/media-files/"+testCatalogFileID+"/subtitles/x:0123456789ab.vtt" {
		t.Errorf("tracks = %+v", tracks)
	}
}

func TestMediaFileDefaultSubtitleMayBeExternal(t *testing.T) {
	t.Parallel()

	lister := &fakeTrackLister{tracks: []playback.SubtitleTrack{
		{ID: "x:aaaaaaaaaaaa", Language: "eng", Format: "subrip", Forced: true, External: true, TextBased: true},
	}}

	english := englishFile(t)
	english.Description.Streams = english.Description.Streams[:2]

	file := getItemFile(t, restapi.StreamingServices{Tracks: lister}, english)

	if file.DefaultSubtitleTrackID == nil || *file.DefaultSubtitleTrackID != "x:aaaaaaaaaaaa" {
		t.Errorf("defaultSubtitleTrackId = %v, want the external forced track", file.DefaultSubtitleTrackID)
	}
}

func TestMediaFileKeepsEmbeddedTracksWhenTheExternalLookupFails(t *testing.T) {
	t.Parallel()

	lister := &fakeTrackLister{err: errors.New("database is locked")}

	file := getItemFile(t, restapi.StreamingServices{Tracks: lister}, englishFile(t))

	if file.SubtitleTracks == nil || len(*file.SubtitleTracks) != 2 {
		t.Errorf("subtitleTracks = %v, want the 2 embedded ones", file.SubtitleTracks)
	}
}

func TestMediaFileWithoutVideoOrAudioOmitsDefaultsAndSize(t *testing.T) {
	t.Parallel()

	file := englishFile(t)
	file.Description.Streams = nil

	got := getItemFile(t, restapi.StreamingServices{}, file)

	if got.Width != nil || got.Height != nil || got.Resolution != nil || got.DefaultAudioStreamIndex != nil ||
		got.DefaultSubtitleTrackID != nil || got.SubtitleTracks == nil || len(*got.SubtitleTracks) != 0 {
		t.Errorf("file = %+v", got)
	}
}
