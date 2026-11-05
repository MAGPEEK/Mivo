package photometa

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/magpeek/mivo/internal/domain/photo"
)

// Fujifilm RAF header: the magic, then the big-endian offset and length of the embedded
// JPEG, which also carries the EXIF block.
const (
	rafMagic        = "FUJIFILMCCD-RAW "
	rafJPEGOffsetAt = 84
	rafJPEGLengthAt = 88
	rafHeaderSize   = 92
)

// Canon CR3 layout (ISOBMFF): moov holds a Canon uuid box whose CMT1, CMT2 and CMT4 boxes
// are TIFF streams for IFD0, the Exif IFD and the GPS IFD; a top-level preview uuid box
// holds the PRVW box with the full-size JPEG preview.
const (
	cr3CanonUUID   = "\x85\xc0\xb6\x87\x82\x0f\x11\xe0\x81\x11\xf4\xce\x46\x2b\x6a\x48"
	cr3PreviewUUID = "\xea\xf4\x2b\x5e\x1c\x98\x4b\x88\xb9\xfb\xb7\xdc\x40\x6e\x4d\x16"
	// cr3PreviewSkip precedes the PRVW box in the preview uuid payload.
	cr3PreviewSkip = 8
	// cr3PRVWLengthAt and cr3PRVWDataAt locate the JPEG length and data in the PRVW payload.
	cr3PRVWLengthAt = 12
	cr3PRVWDataAt   = 16
)

// rafJPEG returns the location of the embedded JPEG of a RAF file.
func rafJPEG(src source) (previewCandidate, error) {
	header := make([]byte, rafHeaderSize)
	if err := src.readAt(header, 0); err != nil {
		return previewCandidate{}, fmt.Errorf("read RAF header: %w", err)
	}

	candidate := previewCandidate{
		src:    src,
		offset: int64(binary.BigEndian.Uint32(header[rafJPEGOffsetAt:])),
		length: int64(binary.BigEndian.Uint32(header[rafJPEGLengthAt:])),
	}

	return candidate, nil
}

func readRAFInfo(ctx context.Context, src source) (photo.Info, error) {
	candidate, err := rafJPEG(src)
	if err != nil {
		return photo.Info{}, err
	}

	embedded, err := src.section(candidate.offset, candidate.length)
	if err != nil {
		return photo.Info{}, fmt.Errorf("RAF JPEG: %w", err)
	}

	layout, err := scanJPEG(ctx, embedded)
	if err != nil {
		return photo.Info{}, fmt.Errorf("scan RAF JPEG: %w", err)
	}

	// The embedded JPEG is a reduced preview, so its frame size is not the photo's size:
	// without Exif pixel dimensions the size stays unknown.
	var builder infoBuilder
	if err := readLayoutExif(ctx, layout, &builder); err != nil {
		return photo.Info{}, err
	}

	return builder.build(), nil
}

func extractRAFPreview(ctx context.Context, src source) ([]byte, error) {
	candidate, err := rafJPEG(src)
	if err != nil {
		return nil, err
	}

	return pickPreview(ctx, []previewCandidate{candidate})
}

// cr3CanonBox returns the Canon uuid box inside moov; without it the layout is not the
// simple one this reader supports.
func cr3CanonBox(ctx context.Context, src source) (box, error) {
	moov, found, err := findBox(ctx, src, src.whole(), boxQuery{kind: "moov"})
	if err != nil {
		return box{}, fmt.Errorf("find CR3 moov: %w", err)
	}

	if !found {
		return box{}, ErrUnsupported
	}

	canon, found, err := findBox(ctx, src, moov.payload(), boxQuery{kind: boxTypeUUID, uuid: cr3CanonUUID})
	if err != nil {
		return box{}, fmt.Errorf("find CR3 Canon box: %w", err)
	}

	if !found {
		return box{}, ErrUnsupported
	}

	return canon, nil
}

func readCR3Info(ctx context.Context, src source) (photo.Info, error) {
	canon, err := cr3CanonBox(ctx, src)
	if err != nil {
		return photo.Info{}, err
	}

	var builder infoBuilder

	steps := []cr3Step{
		{kind: "CMT1", apply: builder.applyIFD0},
		{kind: "CMT2", apply: builder.applyExif},
		{kind: "CMT4", apply: builder.applyGPS},
	}

	for _, step := range steps {
		if err := applyCR3Stream(ctx, src, canon, step); err != nil {
			return photo.Info{}, fmt.Errorf("CR3 %s: %w", step.kind, err)
		}
	}

	return builder.build(), nil
}

// cr3Step applies the first IFD of the TIFF stream in the Canon box of type kind.
type cr3Step struct {
	kind  string
	apply func(*tiffStream, ifd)
}

// applyCR3Stream runs step when its box is present.
func applyCR3Stream(ctx context.Context, src source, canon box, step cr3Step) error {
	tiffBox, found, err := findBox(ctx, src, canon.payload(), boxQuery{kind: step.kind})
	if err != nil || !found {
		return err
	}

	section, err := src.section(tiffBox.payloadStart, tiffBox.payloadEnd-tiffBox.payloadStart)
	if err != nil {
		return err
	}

	stream, err := openTIFF(section)
	if err != nil {
		return err
	}

	directory, err := stream.readIFD(ctx, stream.first)
	if err != nil {
		return err
	}

	step.apply(stream, directory)

	return nil
}

// extractCR3Preview returns the JPEG of the PRVW box.
func extractCR3Preview(ctx context.Context, src source) ([]byte, error) {
	previewBox, found, err := findBox(ctx, src, src.whole(), boxQuery{kind: boxTypeUUID, uuid: cr3PreviewUUID})
	if err != nil {
		return nil, fmt.Errorf("find CR3 preview box: %w", err)
	}

	if !found || previewBox.payloadEnd-previewBox.payloadStart < cr3PreviewSkip {
		return nil, ErrNoPreview
	}

	prvw, found, err := findBox(ctx, src, span{start: previewBox.payloadStart + cr3PreviewSkip, end: previewBox.payloadEnd}, boxQuery{kind: "PRVW"})
	if err != nil {
		return nil, fmt.Errorf("find CR3 PRVW box: %w", err)
	}

	if !found || prvw.payloadEnd-prvw.payloadStart < cr3PRVWDataAt {
		return nil, ErrNoPreview
	}

	lengthField := make([]byte, longSize)
	if err := src.readAt(lengthField, prvw.payloadStart+cr3PRVWLengthAt); err != nil {
		return nil, fmt.Errorf("read PRVW length: %w", err)
	}

	candidate := previewCandidate{
		src:    src,
		offset: prvw.payloadStart + cr3PRVWDataAt,
		length: int64(binary.BigEndian.Uint32(lengthField)),
	}

	if candidate.offset+candidate.length > prvw.payloadEnd {
		return nil, fmt.Errorf("PRVW JPEG of %d bytes overruns its box: %w", candidate.length, ErrCorrupt)
	}

	return pickPreview(ctx, []previewCandidate{candidate})
}
