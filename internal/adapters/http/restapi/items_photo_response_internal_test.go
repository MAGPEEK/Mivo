package restapi

import (
	"testing"
	"time"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/photo"
)

// mappedPhotoTime is the capture time of the mapped photos (a wall-clock reading as UTC).
func mappedPhotoTime() time.Time {
	return time.Date(2024, 7, 14, 9, 30, 15, 0, time.UTC)
}

// fullPhotoDetail is a photo turned by a quarter (orientation 6) with every EXIF value.
func fullPhotoDetail() *applibrary.PhotoDetail {
	return &applibrary.PhotoDetail{
		Info: photo.Info{
			TakenAt: mappedPhotoTime(), Make: "Alpha", Model: "Alpha Q2", Lens: "35 mm f/1.8",
			ExposureSeconds: 0.004, FNumber: 1.8, ISO: 200, FocalLengthMM: 35, Orientation: 6,
			GPS: &photo.Coordinates{Latitude: 54.0924, Longitude: -12.1407}, Width: 6000, Height: 4000,
		},
		TakenAtSource: photo.TakenAtExif, ThumbnailState: photo.ThumbnailReady,
	}
}

// TestItemSummaryResponsePhoto: a still photo card carries its capture time, thumbnail
// state and its tile in the square slot, but no media; a video photo keeps its media.
func TestItemSummaryResponsePhoto(t *testing.T) {
	t.Parallel()

	still := media.File{ID: idFile, Description: media.Description{Container: media.ContainerUnknown}}
	summary := applibrary.ItemSummary{
		Item: mapperItem(idEpisode, library.ItemKindPhoto), PrimaryFile: still,
		Poster: &artwork.Ref{ID: idEpisode, Tag: "tile"}, Photo: fullPhotoDetail(),
	}

	response, err := itemSummaryResponse(summary)
	if err != nil {
		t.Fatalf("itemSummaryResponse: %v", err)
	}

	assertFragments(t, mustMarshal(t, response),
		[]string{`"kind":"photo"`, `"takenAt":"2024-07-14T09:30:15Z"`, `"thumbnailState":"ready"`, `"square":{"id":"` + idEpisode + `"`},
		[]string{`"poster"`, `"media"`, `"make"`})

	summary.PrimaryFile.Description = media.Description{
		Container: media.ContainerMP4, Duration: 12 * time.Second,
		Streams: []media.Stream{{Index: 0, Kind: media.StreamKindVideo, Codec: "h264"}},
	}
	summary.Poster = nil

	response, err = itemSummaryResponse(summary)
	if err != nil {
		t.Fatalf("itemSummaryResponse of a video: %v", err)
	}

	assertFragments(t, mustMarshal(t, response), []string{`"media":{`, `"durationSeconds":12`}, []string{`"square"`})
}

// TestItemSummaryResponsePhotoAlbum: an album card shows its cover photo as square and no
// photo fields.
func TestItemSummaryResponsePhotoAlbum(t *testing.T) {
	t.Parallel()

	summary := applibrary.ItemSummary{
		Item:   mapperItem(idEpisode, library.ItemKindPhotoAlbum),
		Poster: &artwork.Ref{ID: idImage, Tag: "cover"}, ChildCount: 2, LeafCount: 7,
	}

	response, err := itemSummaryResponse(summary)
	if err != nil {
		t.Fatalf("itemSummaryResponse: %v", err)
	}

	assertFragments(t, mustMarshal(t, response),
		[]string{`"kind":"photo_album"`, `"square":{"id":"` + idImage + `"`, `"childCount":2`, `"leafCount":7`},
		[]string{`"poster"`, `"takenAt"`, `"thumbnailState"`})
}

// TestItemDetailResponsePhoto: the detail carries the EXIF block with the displayed size
// (width and height swapped by the quarter turn) and the thumbnail state; values the file
// does not carry are omitted.
func TestItemDetailResponsePhoto(t *testing.T) {
	t.Parallel()

	detail := applibrary.ItemDetail{
		Item:    mapperItem(idEpisode, library.ItemKindPhoto),
		Artwork: map[artwork.Kind]artwork.Ref{artwork.KindPoster: {ID: idEpisode, Tag: "tile"}},
		Photo:   fullPhotoDetail(),
	}

	response, err := itemDetailResponse(detail)
	if err != nil {
		t.Fatalf("itemDetailResponse: %v", err)
	}

	assertFragments(t, mustMarshal(t, response), []string{
		`"photo":{`, `"takenAt":"2024-07-14T09:30:15Z"`, `"takenAtSource":"exif"`, `"make":"Alpha"`, `"model":"Alpha Q2"`,
		`"lens":"35 mm f/1.8"`, `"exposureTime":0.004`, `"fNumber":1.8`, `"iso":200`, `"focalLengthMm":35`,
		`"orientation":6`, `"latitude":54.0924`, `"longitude":-12.1407`, `"width":4000`, `"height":6000`,
		`"thumbnailState":"ready"`, `"square":{"id":"` + idEpisode + `"`,
	}, []string{`"poster"`})

	detail.Photo = &applibrary.PhotoDetail{
		Info: photo.Info{TakenAt: mappedPhotoTime()}, TakenAtSource: photo.TakenAtFile, ThumbnailState: photo.ThumbnailUnsupported,
	}
	detail.Artwork = nil

	response, err = itemDetailResponse(detail)
	if err != nil {
		t.Fatalf("itemDetailResponse without EXIF: %v", err)
	}

	assertFragments(t, mustMarshal(t, response),
		[]string{`"takenAtSource":"file"`, `"thumbnailState":"unsupported"`},
		[]string{`"make"`, `"exposureTime"`, `"iso"`, `"orientation"`, `"latitude"`, `"width"`, `"artwork"`})
}

// TestItemDetailResponseWithoutPhoto: other kinds send neither block.
func TestItemDetailResponseWithoutPhoto(t *testing.T) {
	t.Parallel()

	response, err := itemDetailResponse(applibrary.ItemDetail{Item: mapperItem(idEpisode, library.ItemKindPhotoAlbum)})
	if err != nil {
		t.Fatalf("itemDetailResponse: %v", err)
	}

	assertFragments(t, mustMarshal(t, response), nil, []string{`"photo"`, `"thumbnailState"`})
}
