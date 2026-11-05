package photometa

import (
	"cmp"
	"context"
	"fmt"
	"slices"
)

const (
	// maxPreviewBytes bounds a returned preview; full-size RAW previews are 1-15 MiB.
	maxPreviewBytes = 64 << 20
	// JPEG compression values of TIFF: old-style (6) and new-style/DNG (7).
	compressionOldJPEG = 6
	compressionJPEG    = 7
	// singleStrip is the strip count of a preview stored as one complete JPEG.
	singleStrip = 1
)

// previewCandidate is a byte range of src that may hold a complete JPEG.
type previewCandidate struct {
	src    source
	offset int64
	length int64
}

// previewFromTIFF returns the largest renderable JPEG referenced by the TIFF stream in src.
func previewFromTIFF(ctx context.Context, src source) ([]byte, error) {
	stream, err := openTIFF(src)
	if err != nil {
		return nil, err
	}

	return pickPreview(ctx, collectCandidates(ctx, stream))
}

func extractTIFFFilePreview(ctx context.Context, src source) ([]byte, error) {
	return previewFromTIFF(ctx, src)
}

// collectCandidates walks the IFD chain and the SubIFDs of each IFD. Broken directories
// and a loop in the chain end the walk but keep the candidates found so far.
func collectCandidates(ctx context.Context, stream *tiffStream) []previewCandidate {
	var candidates []previewCandidate

	chain := map[uint32]struct{}{}

	offset := stream.first
	for offset != 0 {
		if _, looped := chain[offset]; looped {
			break
		}

		chain[offset] = struct{}{}

		directory, err := stream.readIFD(ctx, offset)
		if err != nil {
			break
		}

		candidates = append(candidates, directoryCandidates(stream, directory)...)

		for _, subOffset := range stream.uints(directory, tagSubIFDs, maxSubIFDs) {
			sub, err := stream.readIFD(ctx, subOffset)
			if err != nil {
				continue
			}

			candidates = append(candidates, directoryCandidates(stream, sub)...)
		}

		offset = directory.next
	}

	return candidates
}

// directoryCandidates lists the JPEGs one IFD references: JPEGInterchangeFormat, a single
// JPEG-compressed strip and the RW2 JpgFromRaw blob.
func directoryCandidates(stream *tiffStream, directory ifd) []previewCandidate {
	var candidates []previewCandidate

	add := func(offset, length int64) {
		candidates = append(candidates, previewCandidate{src: stream.src, offset: offset, length: length})
	}

	offset, offsetOK := stream.uint(directory, tagJPEGOffset)
	length, lengthOK := stream.uint(directory, tagJPEGLength)

	if offsetOK && lengthOK {
		add(int64(offset), int64(length))
	}

	if compression, ok := stream.uint(directory, tagCompression); ok &&
		(compression == compressionOldJPEG || compression == compressionJPEG) {
		offsets := stream.uints(directory, tagStripOffsets, singleStrip+1)
		lengths := stream.uints(directory, tagStripCounts, singleStrip+1)

		if len(offsets) == singleStrip && len(lengths) == singleStrip {
			add(int64(offsets[0]), int64(lengths[0]))
		}
	}

	if entry, ok := directory.entries[tagRW2JpgFromRaw]; ok && entry.kind == typeUndefined {
		if blobOffset, size, inline := stream.valueLocation(entry); !inline {
			add(blobOffset, size)
		}
	}

	return candidates
}

// pickPreview returns the largest candidate that fits maxPreviewBytes and is a renderable
// JPEG; ErrNoPreview when none is.
func pickPreview(ctx context.Context, candidates []previewCandidate) ([]byte, error) {
	slices.SortStableFunc(candidates, func(left, right previewCandidate) int {
		return cmp.Compare(right.length, left.length)
	})

	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("pick preview: %w", err)
		}

		if candidate.length <= 0 || candidate.length > maxPreviewBytes {
			continue
		}

		section, err := candidate.src.section(candidate.offset, candidate.length)
		if err != nil || !isRenderableJPEG(ctx, section) {
			continue
		}

		return section.readBytes(0, section.size, maxPreviewBytes)
	}

	return nil, ErrNoPreview
}
