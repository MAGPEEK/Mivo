package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

// The media catalog (ADR 0022) is split by use: CatalogWriter serves the scan (change
// detection and its writes, port app/scan.CatalogWriter), CatalogReader serves the library
// grid, the item detail (port app/library.ItemReader) and playback (port
// app/playback.FileSource). Both store paths relative to the library root, never absolute.

// errUndecodableRow marks a stored row the domain rejects: an unknown kind, container or
// HDR format, a malformed frame rate, streams next to a probe error (an edited database or
// a newer version after a downgrade). It deliberately does not wrap domain.ErrInvalid,
// which callers report as a bad request: the fault lies in the server's data, not in the
// client's input.
var errUndecodableRow = errors.New("stored row cannot be decoded")

// undecodable reports a stored value the domain rejected. The domain's error text is kept,
// its chain is not, so domain.ErrInvalid does not leak to the caller.
func undecodable(what string, err error) error {
	return fmt.Errorf("%s: %w: %s", what, errUndecodableRow, err.Error())
}

// Stored flag values of the media_streams disposition columns.
const (
	storedFalse int64 = 0
	storedTrue  int64 = 1
)

// itemFromRow rejects a stored kind the domain does not know instead of handing it to
// clients.
func itemFromRow(row sqlcgen.Item) (library.Item, error) {
	kind, err := library.ParseItemKind(row.Kind)
	if err != nil {
		return library.Item{}, undecodable(fmt.Sprintf("item %q: stored kind", row.ID), err)
	}

	return library.Item{
		ID:        row.ID,
		LibraryID: row.LibraryID,
		Kind:      kind,
		Title:     row.Title,
		SortTitle: row.SortTitle,
		CreatedAt: time.UnixMilli(row.CreatedAt).UTC(),
	}, nil
}

// fileFromRow builds a media.File from its row and its streams in stream order and validates
// it, so a corrupt row (see errUndecodableRow) is reported instead of handed to clients. A
// file whose probe failed has the zero description apart from such stray streams.
func fileFromRow(row sqlcgen.MediaFile, streamRows []sqlcgen.MediaStream) (media.File, error) {
	// The path is handed to the path resolver; one that is not canonical (absolute, "..")
	// never left this adapter's writes, so it can only come from an edited database.
	if _, err := library.ParseRelativePath(row.RelativePath); err != nil {
		return media.File{}, undecodable(fmt.Sprintf("file %q: stored relative path", row.ID), err)
	}

	streams, err := streamsFromRows(streamRows)
	if err != nil {
		return media.File{}, fmt.Errorf("file %q: %w", row.ID, err)
	}

	file := media.File{
		ID:           row.ID,
		ItemID:       row.ItemID,
		LibraryID:    row.LibraryID,
		RelativePath: row.RelativePath,
		Size:         row.SizeBytes,
		ModifiedAt:   time.UnixMilli(row.ModifiedAt).UTC(),
		Description:  media.Description{Streams: streams},
	}

	if row.ProbeError.Valid {
		file.ProbeError = media.ProbeError(row.ProbeError.String)
	} else {
		file.Description.Container = media.Container(row.Container)
		file.Description.FormatName = row.FormatName
		file.Description.Duration = time.Duration(row.DurationMs) * time.Millisecond
		file.Description.BitRate = row.BitRate
	}

	if err := file.Validate(); err != nil {
		return media.File{}, undecodable("stored file", err)
	}

	return file, nil
}

// streamsFromRows returns nil for no rows, the value a probe without streams produces.
func streamsFromRows(rows []sqlcgen.MediaStream) ([]media.Stream, error) {
	var streams []media.Stream

	for _, row := range rows {
		stream, err := streamFromRow(row)
		if err != nil {
			return nil, err
		}

		streams = append(streams, stream)
	}

	return streams, nil
}

// streamFromRow parses the frame rate; kind and HDR format are checked by
// media.File.Validate together with the rest of the file.
func streamFromRow(row sqlcgen.MediaStream) (media.Stream, error) {
	frameRate, err := media.ParseFrameRate(row.FrameRate)
	if err != nil {
		return media.Stream{}, undecodable(fmt.Sprintf("stream %d: stored frame rate", row.StreamIndex), err)
	}

	return media.Stream{
		Index:           int(row.StreamIndex),
		Kind:            media.StreamKind(row.Kind),
		Codec:           row.Codec,
		CodecTag:        row.CodecTag.String,
		Profile:         row.Profile,
		Level:           int(row.Level),
		PixelFormat:     row.PixelFormat,
		BitDepth:        int(row.BitDepth),
		Width:           int(row.Width),
		Height:          int(row.Height),
		FrameRate:       frameRate,
		ColorPrimaries:  row.ColorPrimaries,
		ColorTransfer:   row.ColorTransfer,
		ColorSpace:      row.ColorSpace,
		HDR:             media.HDRFormat(row.Hdr),
		Channels:        int(row.Channels),
		ChannelLayout:   row.ChannelLayout,
		SampleRate:      int(row.SampleRate),
		Language:        row.Language,
		Title:           row.Title,
		Default:         row.IsDefault == storedTrue,
		Forced:          row.IsForced == storedTrue,
		AttachedPicture: row.IsAttachedPicture == storedTrue,
		BitRate:         row.BitRate,
	}, nil
}

// fileParams maps a validated file to its row. A failed probe is stored with the container
// "unknown" and no description values (ADR 0022 notes), whatever the caller put there.
func fileParams(file media.File, probedAt int64) sqlcgen.UpsertMediaFileParams {
	params := sqlcgen.UpsertMediaFileParams{
		ID:           file.ID,
		ItemID:       file.ItemID,
		LibraryID:    file.LibraryID,
		RelativePath: file.RelativePath,
		SizeBytes:    file.Size,
		ModifiedAt:   file.ModifiedAt.UnixMilli(),
		Container:    string(media.ContainerUnknown),
		ProbedAt:     probedAt,
	}

	if !file.Probed() {
		params.ProbeError = sql.NullString{String: string(file.ProbeError), Valid: true}

		return params
	}

	description := file.Description
	params.Container = string(description.Container)
	params.FormatName = description.FormatName
	params.DurationMs = description.Duration.Milliseconds()
	params.BitRate = description.BitRate

	return params
}

// streamParams maps a stream to its row. An empty codec tag is stored as NULL: the container
// stores no tag.
func streamParams(fileID string, stream media.Stream) sqlcgen.InsertStreamParams {
	return sqlcgen.InsertStreamParams{
		MediaFileID:       fileID,
		StreamIndex:       int64(stream.Index),
		Kind:              string(stream.Kind),
		Codec:             stream.Codec,
		CodecTag:          sql.NullString{String: stream.CodecTag, Valid: stream.CodecTag != ""},
		Profile:           stream.Profile,
		Level:             int64(stream.Level),
		PixelFormat:       stream.PixelFormat,
		BitDepth:          int64(stream.BitDepth),
		Width:             int64(stream.Width),
		Height:            int64(stream.Height),
		FrameRate:         stream.FrameRate.String(),
		ColorPrimaries:    stream.ColorPrimaries,
		ColorTransfer:     stream.ColorTransfer,
		ColorSpace:        stream.ColorSpace,
		Hdr:               string(stream.HDR),
		Channels:          int64(stream.Channels),
		ChannelLayout:     stream.ChannelLayout,
		SampleRate:        int64(stream.SampleRate),
		Language:          stream.Language,
		Title:             stream.Title,
		IsDefault:         storedFlag(stream.Default),
		IsForced:          storedFlag(stream.Forced),
		IsAttachedPicture: storedFlag(stream.AttachedPicture),
		BitRate:           stream.BitRate,
	}
}

func storedFlag(value bool) int64 {
	if value {
		return storedTrue
	}

	return storedFalse
}
