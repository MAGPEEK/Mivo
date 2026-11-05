package photo

import "time"

// ThumbnailState says whether a thumbnail of a photo can be rendered (API thumbnailState).
type ThumbnailState string

// Known thumbnail states.
const (
	// ThumbnailReady means the renderer is expected to produce a thumbnail; a render that
	// still fails is reported by the image endpoint, not stored by the scan.
	ThumbnailReady ThumbnailState = "ready"
	// ThumbnailUnsupported means the file is listed but no thumbnail can be rendered.
	ThumbnailUnsupported ThumbnailState = "unsupported"
)

// ThumbnailStateFor decides the thumbnail state of a photo file from its name and whether
// its metadata reader reported the file's layout as unsupported. Only RAW files depend on
// that reader: they are shown through the embedded preview it extracts (ADR 0029), so a RAW
// layout it cannot read has no thumbnail. Every other image (and video) is decoded by FFmpeg
// directly, whatever the metadata reader can parse, so a PNG without EXIF stays ready.
func ThumbnailStateFor(name string, metadataUnsupported bool) ThumbnailState {
	if metadataUnsupported && isRAW(name) {
		return ThumbnailUnsupported
	}

	return ThumbnailReady
}

// WallClock returns the wall-clock reading of instant in zone as a UTC time, the time
// reference of Info.TakenAt. The scan uses it for the modification time of a file without
// an EXIF capture time, so both sort on the same scale (the server's local time stands in
// for the camera's). A nil zone means UTC.
func WallClock(instant time.Time, zone *time.Location) time.Time {
	if zone == nil {
		zone = time.UTC
	}

	local := instant.In(zone)

	return time.Date(local.Year(), local.Month(), local.Day(),
		local.Hour(), local.Minute(), local.Second(), local.Nanosecond(), time.UTC)
}
