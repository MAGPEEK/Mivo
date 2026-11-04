package artwork

import "time"

// WholeFile is the stream index of a location that is a whole image file rather than an
// attached-picture stream of a media file.
const WholeFile = -1

// Location identifies the bytes of one picture, depending on its source:
//   - local: RelativePath of the image file inside the library, StreamIndex WholeFile;
//   - embedded: RelativePath of the media file inside the library and the StreamIndex of
//     its attached-picture stream;
//   - provider and upload: DataPath relative to the data directory, StreamIndex WholeFile.
//
// Relative paths use the canonical form of library.ParseRelativePath. Use the constructors
// so StreamIndex is never left at the valid stream index 0 by accident.
//
// RootID names the library folder of a local or embedded location (ADR 0028); it is empty
// for data-directory pictures and for catalogs without folders. It is not part of the cache
// tag: the tag only has to change when the picture's bytes change.
type Location struct {
	RootID       string
	RelativePath string
	StreamIndex  int
	DataPath     string
}

// LocalLocation returns the location of an image file inside a library.
func LocalLocation(relativePath string) Location {
	return Location{RelativePath: relativePath, StreamIndex: WholeFile}
}

// EmbeddedLocation returns the location of an attached-picture stream of a media file.
func EmbeddedLocation(mediaRelativePath string, streamIndex int) Location {
	return Location{RelativePath: mediaRelativePath, StreamIndex: streamIndex}
}

// DataLocation returns the location of a file in the data directory.
func DataLocation(dataPath string) Location {
	return Location{DataPath: dataPath, StreamIndex: WholeFile}
}

// Candidate is a picture a scan found for one kind of one item. Priority orders local
// candidates (lower first, see ClassifyLocal); Size and ModifiedAt describe the file the
// picture is read from and feed the cache tag.
type Candidate struct {
	Kind       Kind
	Source     SourceKind
	Priority   int
	Location   Location
	Size       int64
	ModifiedAt time.Time
}

// Tag returns the cache tag of the candidate (see Tag).
func (candidate Candidate) Tag() string {
	return Tag(TagInput{
		Source:     candidate.Source,
		Location:   candidate.Location,
		Size:       candidate.Size,
		ModifiedAt: candidate.ModifiedAt,
	})
}

// Stored is one artwork as the catalog keeps it. LibraryRoot is the absolute root of the
// library for local and embedded pictures and empty for data-directory pictures; the image
// use case resolves Location against it, confined to the root.
type Stored struct {
	ID          string
	Kind        Kind
	Source      SourceKind
	LibraryRoot string
	Location    Location
	Size        int64
	ModifiedAt  time.Time
	Tag         string
}

// Ref returns the API reference of the stored artwork.
func (stored Stored) Ref() Ref {
	return Ref{ID: stored.ID, Tag: stored.Tag}
}

// RenderRequest asks the renderer for one picture. AbsolutePath is an already resolved and
// confined path; StreamIndex is WholeFile for an image file or the attached-picture stream
// of a media file. The renderer never upscales beyond the source width.
type RenderRequest struct {
	AbsolutePath string
	StreamIndex  int
	MaxWidth     Width
	Format       Format
}

// Rendered is an encoded picture with its actual pixel size.
type Rendered struct {
	Data   []byte
	Width  int
	Height int
	Format Format
}
