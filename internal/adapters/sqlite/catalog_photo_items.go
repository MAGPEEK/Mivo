package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/photo"
)

// The generic item reads (listings, children, detail) complete photos and photo albums with
// what their tiles and the lightbox need (ADR 0029): the photo details and the 1:1 tile,
// which the summary and the detail carry in the poster slot, as for music albums. A photo
// is the picture of itself, so its tile references the photo item; an album shows its
// newest still photo whose thumbnail is ready.

// addPhotoSummaries completes the summaries of photos and photo albums in place; other
// kinds are left alone. It costs one query for the photos of the page and two per album.
func addPhotoSummaries(ctx context.Context, queries *sqlcgen.Queries, summaries []applibrary.ItemSummary) error {
	details, err := photoDetailsOf(ctx, queries, summaries)
	if err != nil {
		return err
	}

	for index := range summaries {
		summary := &summaries[index]

		if summary.Item.Kind == library.ItemKindPhoto {
			if detail, found := details[summary.Item.ID]; found {
				summary.Photo = &detail
			}

			summary.Poster = photoTile(summary.Item.ID, summary.Photo, summary.PrimaryFile)
		}

		if summary.Item.Kind != library.ItemKindPhotoAlbum {
			continue
		}

		if err := addAlbumTotals(ctx, queries, summary); err != nil {
			return err
		}
	}

	return nil
}

// photoDetailsOf reads the photo details of the photos among summaries, keyed by item ID.
func photoDetailsOf(ctx context.Context, queries *sqlcgen.Queries, summaries []applibrary.ItemSummary) (map[string]applibrary.PhotoDetail, error) {
	var photoIDs []string

	for _, summary := range summaries {
		if summary.Item.Kind == library.ItemKindPhoto {
			photoIDs = append(photoIDs, summary.Item.ID)
		}
	}

	if len(photoIDs) == 0 {
		return map[string]applibrary.PhotoDetail{}, nil
	}

	rows, err := queries.ListPhotoInfos(ctx, photoIDs)
	if err != nil {
		return nil, fmt.Errorf("read photo details: %w", err)
	}

	details := make(map[string]applibrary.PhotoDetail, len(rows))

	for _, row := range rows {
		detail, err := photoDetailFromRow(row)
		if err != nil {
			return nil, err
		}

		details[row.ItemID] = detail
	}

	return details, nil
}

// addAlbumTotals sets the photo count of an album with its sub-albums and its tile.
func addAlbumTotals(ctx context.Context, queries *sqlcgen.Queries, summary *applibrary.ItemSummary) error {
	totals, err := queries.GetPhotoAlbumTotals(ctx, summary.Item.ID)
	if err != nil {
		return fmt.Errorf("read totals of album %q: %w", summary.Item.ID, err)
	}

	summary.LeafCount = int(totals.LeafCount)

	summary.Poster, err = albumTile(ctx, queries, totals.CoverPhotoID)

	return err
}

// albumTile references the cover photo of an album; none for an empty coverPhotoID.
func albumTile(ctx context.Context, queries *sqlcgen.Queries, coverPhotoID string) (*artwork.Ref, error) {
	if coverPhotoID == "" {
		return nil, nil //nolint:nilnil // nil pointer is the "no tile" value
	}

	file, err := queries.GetPhotoMainFile(ctx, coverPhotoID)
	if errors.Is(err, sql.ErrNoRows) {
		// GetPhotoAlbumTotals found the photo in the same read transaction, with its file.
		return nil, undecodable(fmt.Sprintf("cover photo %q", coverPhotoID), err)
	}

	if err != nil {
		return nil, fmt.Errorf("read file of cover photo %q: %w", coverPhotoID, err)
	}

	tag := artwork.PhotoTag(file.RelativePath, file.SizeBytes, time.UnixMilli(file.ModifiedAt))

	return &artwork.Ref{ID: coverPhotoID, Tag: tag}, nil
}

// photoTile references a photo as its own tile: only a still photo whose thumbnail can be
// rendered has one. A video has none yet, because the image endpoint renders still images
// only, and neither has a photo the scan has not detailed.
func photoTile(itemID string, detail *applibrary.PhotoDetail, file media.File) *artwork.Ref {
	if detail == nil || detail.ThumbnailState != photo.ThumbnailReady || file.ID == "" || isVideoFile(file) {
		return nil
	}

	return &artwork.Ref{ID: itemID, Tag: artwork.PhotoTag(file.RelativePath, file.Size, file.ModifiedAt)}
}

// isVideoFile tells a video in a photos library from a still photo, which the scan saves
// without streams.
func isVideoFile(file media.File) bool {
	_, found := file.Description.PrimaryVideo()

	return found
}

// readDetailPhoto completes the detail of a photo or photo album; other kinds are left alone.
func readDetailPhoto(ctx context.Context, queries *sqlcgen.Queries, row sqlcgen.Item, detail *applibrary.ItemDetail) error {
	switch row.Kind {
	case library.ItemKindPhoto.String():
		return readPhotoItemDetail(ctx, queries, row, detail)
	case library.ItemKindPhotoAlbum.String():
		return readPhotoAlbumDetail(ctx, queries, row, detail)
	default:
		return nil
	}
}

// readPhotoItemDetail adds the photo details and the tile. Play starts only a video; a
// photo the scan has not detailed yet has neither block nor tile.
func readPhotoItemDetail(ctx context.Context, queries *sqlcgen.Queries, row sqlcgen.Item, detail *applibrary.ItemDetail) error {
	var primary media.File
	if len(detail.Files) > 0 {
		primary = detail.Files[0]
	}

	if !isVideoFile(primary) {
		detail.PlayItemID = ""
	}

	infoRow, err := queries.GetPhotoInfo(ctx, row.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("read photo details: %w", err)
	}

	photoDetail, err := photoDetailFromRow(infoRow)
	if err != nil {
		return err
	}

	detail.Photo = &photoDetail

	if tile := photoTile(row.ID, &photoDetail, primary); tile != nil {
		detail.Artwork[artwork.KindPoster] = *tile
	}

	return nil
}

// readPhotoAlbumDetail adds the photo count with the sub-albums and the tile; an album has
// nothing to play.
func readPhotoAlbumDetail(ctx context.Context, queries *sqlcgen.Queries, _ sqlcgen.Item, detail *applibrary.ItemDetail) error {
	detail.PlayItemID = ""

	summary := applibrary.ItemSummary{Item: detail.Item}
	if err := addAlbumTotals(ctx, queries, &summary); err != nil {
		return err
	}

	detail.LeafCount = summary.LeafCount
	if summary.Poster != nil {
		detail.Artwork[artwork.KindPoster] = *summary.Poster
	}

	return nil
}

// photoChildrenPage reads one page of the children of a photo album (see readPhotoChildren)
// as summaries.
func photoChildrenPage(ctx context.Context, queries *sqlcgen.Queries, request photoChildrenRequest) (applibrary.ItemPage, error) {
	page, err := readPhotoChildren(ctx, queries, request)
	if err != nil {
		return applibrary.ItemPage{}, err
	}

	itemIDs := make([]string, 0, len(page.Children))
	for _, child := range page.Children {
		itemIDs = append(itemIDs, child.Item.ID)
	}

	primaryFiles, err := primaryFilesByItem(ctx, queries, itemIDs)
	if err != nil {
		return applibrary.ItemPage{}, err
	}

	summaries := make([]applibrary.ItemSummary, 0, len(page.Children))

	for _, child := range page.Children {
		summary := applibrary.ItemSummary{
			Item: child.Item, PrimaryFile: primaryFiles[child.Item.ID],
			ChildCount: child.ChildCount, LeafCount: child.LeafCount, Photo: child.Photo,
		}

		if child.Photo != nil {
			summary.Poster = photoTile(child.Item.ID, child.Photo, summary.PrimaryFile)
		} else if summary.Poster, err = albumTile(ctx, queries, child.CoverPhotoID); err != nil {
			return applibrary.ItemPage{}, err
		}

		summaries = append(summaries, summary)
	}

	return applibrary.ItemPage{Items: summaries, NextCursor: page.NextCursor}, nil
}
