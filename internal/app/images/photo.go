package images

import (
	"context"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/domain/artwork"
)

// PhotoFile is the main file of a photo item: the picture its thumbnails are rendered from
// (ADR 0029). LibraryRoot is the absolute root of the library folder; RelativePath, Size and
// ModifiedAt describe the file as the last scan saw it.
type PhotoFile struct {
	ItemID       string
	LibraryRoot  string
	RelativePath string
	Size         int64
	ModifiedAt   time.Time
}

// Tag returns the cache tag of the photo's thumbnails (artwork.PhotoTag): it changes with
// the file's modification time and size, so an edited photo is rendered again.
func (file PhotoFile) Tag() string {
	return artwork.PhotoTag(file.RelativePath, file.Size, file.ModifiedAt)
}

// PhotoSource reads photo items for their thumbnails: a photo is the artwork of itself, so
// the image endpoint accepts a photo item ID. Implemented by adapters/sqlite.
type PhotoSource interface {
	// PhotoFile returns the main file of a photo item. An ID that names no photo wraps
	// domain.ErrNotFound.
	PhotoFile(ctx context.Context, itemID string) (PhotoFile, error)
	// PhotoTags returns PhotoFile.Tag of every photo, keyed by item ID.
	PhotoTags(ctx context.Context) (map[string]string, error)
}

// photoTarget is the render target of a photo's thumbnail. Photos are always JPEG: they
// carry no transparency worth the size of PNG.
func photoTarget(file PhotoFile, width artwork.Width, renderer Renderer) renderTarget {
	return renderTarget{
		key:         Key{ID: file.ItemID, Tag: file.Tag(), Width: width, Format: artwork.FormatJPEG},
		libraryRoot: file.LibraryRoot,
		location:    artwork.LocalLocation(file.RelativePath),
		renderer:    renderer,
	}
}

// photoTags returns the current tags of every photo, or none without a photo source.
func (service *Service) photoTags(ctx context.Context) (map[string]string, error) {
	if service.dependencies.Photos == nil {
		return map[string]string{}, nil
	}

	tags, err := service.dependencies.Photos.PhotoTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("read photo tags: %w", err)
	}

	return tags, nil
}
