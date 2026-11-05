package restapi

import (
	"fmt"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/photo"
)

// applyPhotoSummary sets the capture time and thumbnail state of a photo card; other kinds
// carry no photo details.
func applyPhotoSummary(response *apigen.ItemSummary, summary applibrary.ItemSummary) error {
	if summary.Photo == nil {
		return nil
	}

	state, err := thumbnailStateResponse(summary.Photo.ThumbnailState)
	if err != nil {
		return fmt.Errorf("item %s: %w", summary.Item.ID, err)
	}

	takenAt := apiTimestamp(summary.Photo.Info.TakenAt)
	response.TakenAt, response.ThumbnailState = &takenAt, &state

	return nil
}

// showsMediaSummary reports whether a listing sends the media summary of the primary file:
// a still photo has none (its image file has no streams), so media-based filters and the
// duration badge apply to videos only.
func showsMediaSummary(summary applibrary.ItemSummary) bool {
	if !summary.HasPrimaryFile() {
		return false
	}

	if summary.Item.Kind != library.ItemKindPhoto {
		return true
	}

	_, isVideo := summary.PrimaryFile.Description.PrimaryVideo()

	return isVideo
}

// applyPhoto sends the EXIF block and the thumbnail state of a photo (ADR 0029).
func applyPhoto(response *apigen.ItemDetail, detail applibrary.ItemDetail) error {
	if detail.Photo == nil {
		return nil
	}

	state, err := thumbnailStateResponse(detail.Photo.ThumbnailState)
	if err != nil {
		return err
	}

	block, err := itemPhotoResponse(*detail.Photo)
	if err != nil {
		return err
	}

	response.Photo, response.ThumbnailState = &block, &state

	return nil
}

// itemPhotoResponse maps the capture data; values the file does not carry (zero) are
// omitted, the size is sent as displayed (orientation applied).
func itemPhotoResponse(detail applibrary.PhotoDetail) (apigen.ItemPhoto, error) {
	source, err := photoTimeSourceResponse(detail.TakenAtSource)
	if err != nil {
		return apigen.ItemPhoto{}, err
	}

	info := detail.Info
	width, height := info.DisplaySize()
	block := apigen.ItemPhoto{
		TakenAt: apiTimestamp(info.TakenAt), TakenAtSource: source,
		Make: optionalString(info.Make), Model: optionalString(info.Model), Lens: optionalString(info.Lens),
		ExposureTime: optionalPositive(info.ExposureSeconds), FNumber: optionalPositive(info.FNumber),
		FocalLengthMm: optionalPositive(info.FocalLengthMM), Iso: optionalInt32(info.ISO),
		Orientation: optionalInt32(info.Orientation), Width: optionalInt32(width), Height: optionalInt32(height),
	}

	if info.GPS != nil {
		latitude, longitude := info.GPS.Latitude, info.GPS.Longitude
		block.Latitude, block.Longitude = &latitude, &longitude
	}

	return block, nil
}

// optionalPositive omits 0, which photo.Info uses for "not in the file".
func optionalPositive(value float64) *float64 {
	if value <= 0 {
		return nil
	}

	return &value
}

func thumbnailStateResponse(state photo.ThumbnailState) (apigen.ThumbnailState, error) {
	switch state {
	case photo.ThumbnailReady:
		return apigen.ThumbnailStateReady, nil
	case photo.ThumbnailUnsupported:
		return apigen.ThumbnailStateUnsupported, nil
	default:
		return "", fmt.Errorf("thumbnail state %q not in the contract", state)
	}
}

func photoTimeSourceResponse(source photo.TakenAtSource) (apigen.PhotoTimeSource, error) {
	switch source {
	case photo.TakenAtExif:
		return apigen.PhotoTimeSourceExif, nil
	case photo.TakenAtFile:
		return apigen.PhotoTimeSourceFile, nil
	default:
		return "", fmt.Errorf("capture time source %q not in the contract", source)
	}
}

// isPhotoKind reports the kinds of a photos library, whose tile is square (see
// showsSquareArtwork).
func isPhotoKind(kind library.ItemKind) bool {
	return kind == library.ItemKindPhoto || kind == library.ItemKindPhotoAlbum
}
