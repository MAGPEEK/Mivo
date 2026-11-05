package ffmpeg

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
)

const (
	decodersQuery = "-decoders"
	demuxersQuery = "-demuxers"

	// demuxingFlag marks a format FFmpeg can read.
	demuxingFlag = 'D'
	// demuxerISOBMFF reads HEIF and AVIF still images (items and grids since FFmpeg 7.1).
	demuxerISOBMFF = "mov"
	// decoderHEVC decodes the HEVC coded images of HEIC files.
	decoderHEVC = "hevc"
)

// av1SoftwareDecoders decode AV1 without hardware. FFmpeg's own "av1" decoder is missing on
// purpose: it decodes only through a hardware accelerator, which a NAS rarely has.
func av1SoftwareDecoders() []string {
	return []string{"libdav1d", "libaom-av1"}
}

// PhotoDecoders says which photo formats beyond JPEG, PNG, WebP, GIF and TIFF the ffmpeg
// executable decodes. The zero value is a build without them: HEIC and AVIF photos then
// fall back to their embedded preview (ADR 0029).
type PhotoDecoders struct {
	HEIC bool
	AVIF bool
}

// DetectPhotoDecoders asks the ffmpeg executable at ffmpegPath for its decoders and
// demuxers once at startup. On any error the result is the zero value, so photos still work
// through their previews; the error wraps domain.ErrUnavailable (or the caller's context
// error) and is logged here once at warn level, like DetectCapabilities.
func DetectPhotoDecoders(ctx context.Context, ffmpegPath string, logger *slog.Logger) (PhotoDecoders, error) {
	if logger == nil {
		return PhotoDecoders{}, fmt.Errorf("detect photo decoders: logger is nil: %w", domain.ErrInvalid)
	}

	ffmpeg := tool{name: ffmpegToolName, executable: ffmpegPath}

	decoders, err := detectPhotoDecoders(ctx, ffmpeg.output)
	if err != nil && ctx.Err() == nil {
		attributes := append([]slog.Attr{slog.Any("error", err)}, exitAttributes(err)...)
		logger.LogAttrs(ctx, slog.LevelWarn,
			"ffmpeg photo decoder detection failed: HEIC and AVIF photos use their embedded previews", attributes...)
	}

	return decoders, err
}

// detectPhotoDecoders runs both listings under the capability detection timeout.
func detectPhotoDecoders(ctx context.Context, run commandOutput) (PhotoDecoders, error) {
	detectContext, cancel := context.WithTimeout(ctx, capabilityDetectionTimeout)
	defer cancel()

	listings := make(map[string][]listingEntry, len(photoQueries()))

	for _, query := range photoQueries() {
		output, err := run(detectContext, []string{hideBannerFlag, query})
		if err != nil {
			return PhotoDecoders{}, fmt.Errorf("detect photo decoders: ffmpeg %s: %w",
				query, unavailableUnlessCancelled(ctx, err))
		}

		entries, err := parseListing(output)
		if err != nil {
			return PhotoDecoders{}, fmt.Errorf("detect photo decoders: parse ffmpeg %s: %w: %w",
				query, domain.ErrUnavailable, err)
		}

		listings[query] = entries
	}

	return photoDecodersFrom(listings[decodersQuery], listings[demuxersQuery]), nil
}

func photoQueries() []string {
	return []string{decodersQuery, demuxersQuery}
}

// photoDecodersFrom combines the listings: both formats need the ISOBMFF demuxer and a
// non-experimental software decoder of their codec.
func photoDecodersFrom(decoders, demuxers []listingEntry) PhotoDecoders {
	if !hasDemuxer(demuxers, demuxerISOBMFF) {
		return PhotoDecoders{}
	}

	return PhotoDecoders{
		HEIC: hasCodec(decoders, decoderHEVC, encoderKindVideo),
		AVIF: slices.ContainsFunc(av1SoftwareDecoders(), func(name string) bool {
			return hasCodec(decoders, name, encoderKindVideo)
		}),
	}
}

// hasDemuxer reports a format listed under name that FFmpeg can read.
func hasDemuxer(entries []listingEntry, name string) bool {
	return slices.ContainsFunc(entries, func(entry listingEntry) bool {
		return strings.ContainsRune(entry.flags, demuxingFlag) && slices.Contains(entry.names, name)
	})
}
