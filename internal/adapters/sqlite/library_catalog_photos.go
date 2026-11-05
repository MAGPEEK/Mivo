package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/photo"
)

// SavePhotoDetails stores the photo details of a photo item whose files SavePlacedFile saved
// before, in one write transaction; saving again replaces them. Info.TakenAt (the capture
// time, which the scan falls back to the modification time for) and TakenAtSource are stored
// as given; the photo is ordered by the capture time, ties by the lower-case base name of the
// primary file. A zero capture time or an unknown source is invalid. Errors wrap
// domain.ErrNotFound for an unknown item or primary file and domain.ErrInvalid for invalid
// details, an item that is no photo or a primary file of another item; either way nothing
// changes.
func (catalog *LibraryCatalog) SavePhotoDetails(ctx context.Context, details scan.PhotoDetails) error {
	if err := checkPhotoDetails(details); err != nil {
		return fmt.Errorf("save details of photo %q: %w", details.ItemID, err)
	}

	err := inWriteTransaction(ctx, catalog.writer, func(queries *sqlcgen.Queries) error {
		return writePhotoDetails(ctx, queries, details)
	})
	if err != nil {
		return fmt.Errorf("save details of photo %q: %w", details.ItemID, err)
	}

	return nil
}

// checkPhotoDetails rejects what would otherwise surface as a constraint error.
func checkPhotoDetails(details scan.PhotoDetails) error {
	switch {
	case details.ItemID == "":
		return fmt.Errorf("empty item id: %w", domain.ErrInvalid)
	case details.PrimaryFileID == "":
		return fmt.Errorf("empty primary file id: %w", domain.ErrInvalid)
	}

	if _, err := photo.ParseThumbnailState(details.ThumbnailState.String()); err != nil {
		return err
	}

	if _, err := photo.ParseTakenAtSource(details.TakenAtSource.String()); err != nil {
		return err
	}

	if details.Info.TakenAt.IsZero() {
		return fmt.Errorf("zero capture time: %w", domain.ErrInvalid)
	}

	return details.Info.Validate()
}

func writePhotoDetails(ctx context.Context, queries *sqlcgen.Queries, details scan.PhotoDetails) error {
	item, err := itemOfKind(ctx, queries, details.ItemID, library.ItemKindPhoto)
	if err != nil {
		return err
	}

	file, err := queries.GetFileWithRoot(ctx, details.PrimaryFileID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("primary file %q: %w", details.PrimaryFileID, domain.ErrNotFound)
	}

	if err != nil {
		return fmt.Errorf("read primary file %q: %w", details.PrimaryFileID, err)
	}

	if file.MediaFile.ItemID != item.ID {
		return fmt.Errorf("primary file %q belongs to another item: %w", details.PrimaryFileID, domain.ErrInvalid)
	}

	if err := queries.UpsertPhotoInfo(ctx, photoInfoParams(item, file.MediaFile, details)); err != nil {
		return fmt.Errorf("write photo info: %w", err)
	}

	return nil
}

// photoInfoParams builds the photo_info row of item with its primary file.
func photoInfoParams(item sqlcgen.Item, file sqlcgen.MediaFile, details scan.PhotoDetails) sqlcgen.UpsertPhotoInfoParams {
	info := details.Info
	params := sqlcgen.UpsertPhotoInfoParams{
		ItemID: item.ID, LibraryID: item.LibraryID, ParentID: item.ParentID,
		PrimaryFileID: sql.NullString{String: file.ID, Valid: true},
		NameKey:       photoNameKey(file.RelativePath),
		TakenAt:       info.TakenAt.UnixMilli(), TakenAtSource: details.TakenAtSource.String(),
		Make: info.Make, Model: info.Model, Lens: info.Lens,
		Exposure: info.ExposureSeconds, FNumber: info.FNumber, Iso: int64(info.ISO), FocalMm: info.FocalLengthMM,
		Orientation: int64(info.Orientation), Width: int64(info.Width), Height: int64(info.Height),
		ThumbnailState: details.ThumbnailState.String(),
	}

	if gps := info.GPS; gps != nil {
		params.Lat = sql.NullFloat64{Float64: gps.Latitude, Valid: true}
		params.Lon = sql.NullFloat64{Float64: gps.Longitude, Valid: true}
	}

	return params
}

// photoNameKey is the second sort key of a photo: the base name of its file, lower-cased like
// photo.Sort compares names.
func photoNameKey(relativePath string) string {
	return strings.ToLower(path.Base(relativePath))
}
