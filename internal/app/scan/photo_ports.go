package scan

import (
	"context"
	"time"

	"github.com/magpeek/mivo/internal/domain/photo"
)

// PhotoMetadataReader reads the EXIF metadata of a photo. Implemented by adapters/photometa
// ((*Reader).ReadInfo, M3-15c).
type PhotoMetadataReader interface {
	// ReadInfo returns the metadata of the image at absolutePath, which the scan resolved
	// inside a library folder. A file without EXIF yields a zero Info and no error. TakenAt is
	// the camera's wall-clock reading as UTC. Errors wrap context.Canceled or
	// context.DeadlineExceeded when ctx ends, domain.ErrUnsupported for a format or layout
	// the reader cannot parse (a PNG, an unknown RAW variant) and domain.ErrInvalid for broken
	// metadata; any error only costs the photo its metadata.
	ReadInfo(ctx context.Context, absolutePath string) (photo.Info, error)
}

// PhotoDetails is what the scan stores about one photo item beyond its files.
type PhotoDetails struct {
	// ItemID is the photo item, the leaf of the chain SavePlacedFile returned.
	ItemID string
	// PrimaryFileID is the file the photo is shown from: the JPEG of a RAW+JPEG pair, else
	// the only file. Further files of the item (the RAW) are alternates.
	PrimaryFileID string
	// Info is the metadata as read, except TakenAt, which is the capture time: the EXIF time,
	// or the file's modification time as the server's wall clock in UTC when the file has
	// none (photo.CaptureTime, photo.WallClock), so the photo always sorts. Width and Height
	// are the stored pixel size; the API's displayed size is Info.DisplaySize(). A video
	// carries only TakenAt and the size of its primary video stream.
	Info photo.Info
	// ThumbnailState says whether a thumbnail can be rendered (photo.ThumbnailStateFor).
	ThumbnailState photo.ThumbnailState
}

// PhotoCatalog stores the details of photo items. Implemented by adapters/sqlite (M3-15e).
type PhotoCatalog interface {
	// SavePhotoDetails replaces the details of a photo item saved by
	// PlacedFileCatalog.SavePlacedFile; they go when the item goes. Errors wrap
	// domain.ErrNotFound for an unknown item or file, domain.ErrInvalid for an item that is
	// no photo, a primary file of another item or an Info that fails Validate. Errors must
	// not contain absolute paths.
	SavePhotoDetails(ctx context.Context, details PhotoDetails) error
}

// PhotoPorts are the ports only the photo pipeline needs; videos in a photos library use
// the prober of every library.
type PhotoPorts struct {
	Metadata PhotoMetadataReader
	Catalog  PhotoCatalog
	// Zone is the server's time zone for the modification time of photos without an EXIF
	// capture time (photo.WallClock); nil means UTC. Wired as time.Local in cmd/mivo.
	Zone *time.Location
}

func (ports PhotoPorts) complete() bool {
	return ports.Metadata != nil && ports.Catalog != nil
}
