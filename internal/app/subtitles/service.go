package subtitles

import (
	"context"
	"crypto/sha1" //nolint:gosec // names a cache file; it protects nothing
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

const (
	// maxConcurrentConversions keeps subtitle conversions from starving playback on a weak NAS.
	maxConcurrentConversions = 2
	// MaxCacheBytes caps the subtitle cache; the oldest entries go first (ADR 0030 §3).
	MaxCacheBytes int64 = 64 << 20
	// cacheDirectoryName is the folder below the cache directory that holds the WebVTT files.
	cacheDirectoryName = "subtitles"
	cacheExtension     = ".vtt"
	tempExtension      = ".tmp"
	// staleTempAge is when a *.tmp file counts as the leftover of a crashed conversion; a live
	// conversion finishes within the converter's 30 s timeout.
	staleTempAge = 10 * time.Minute
	// digestLength is the number of hex digits of the track and stamp digests in a file name.
	digestLength = 16
)

// Dependencies are the ports the service needs. Logger and Now may be nil. CacheDir is the
// absolute cache directory; the service writes below <CacheDir>/subtitles only.
type Dependencies struct {
	Files     FileSource
	External  ExternalSubtitles
	Resolver  PathResolver
	Statter   Statter
	Converter Converter
	Cache     Cache
	CacheDir  string
	Logger    *slog.Logger
	Now       func() time.Time
}

// WebVTT is a converted subtitle ready to be served. The caller closes Content.
type WebVTT struct {
	Content io.ReadSeekCloser
	Size    int64
	ModTime time.Time
}

// Service serves WebVTT subtitles.
type Service struct {
	dependencies Dependencies
	flights      *flightGroup
	trimMutex    sync.Mutex
}

// NewService returns a service using the given ports. Errors wrap domain.ErrInvalid for a
// missing port or a relative cache directory.
func NewService(dependencies Dependencies) (*Service, error) {
	if dependencies.Files == nil || dependencies.External == nil || dependencies.Resolver == nil ||
		dependencies.Statter == nil || dependencies.Converter == nil || dependencies.Cache == nil {
		return nil, fmt.Errorf("new subtitle service: a port is nil: %w", domain.ErrInvalid)
	}

	if !filepath.IsAbs(dependencies.CacheDir) {
		return nil, fmt.Errorf("new subtitle service: cache directory %q is not absolute: %w",
			dependencies.CacheDir, domain.ErrInvalid)
	}

	if dependencies.Logger == nil {
		dependencies.Logger = slog.New(slog.DiscardHandler)
	}

	if dependencies.Now == nil {
		dependencies.Now = time.Now
	}

	return &Service{dependencies: dependencies, flights: newFlightGroup(maxConcurrentConversions)}, nil
}

// source is where a track is read from and what its cache stamp is made of.
type source struct {
	path  string
	track Track
	stamp []string
}

// WebVTT returns the track of the file as WebVTT, from the cache when the source is
// unchanged. Errors wrap domain.ErrUnauthenticated without a viewer, domain.ErrNotFound for an
// unknown or hidden file, an unknown track or a vanished source, domain.ErrInvalid for a
// picture subtitle, and what the converter reports.
func (service *Service) WebVTT(ctx context.Context, fileID, trackID string) (WebVTT, error) {
	restriction, err := access.RestrictionFrom(ctx)
	if err != nil {
		return WebVTT{}, fmt.Errorf("subtitle %s of %s: %w", trackID, fileID, err)
	}

	playable, err := service.dependencies.Files.PlayableFile(ctx, fileID, restriction)
	if err != nil {
		return WebVTT{}, fmt.Errorf("subtitle %s of %s: %w", trackID, fileID, err)
	}

	// Defense in depth: the source already hides, but a wrong answer must still fail closed.
	if err := access.CheckLibrary(restriction, playable.File.LibraryID); err != nil {
		return WebVTT{}, fmt.Errorf("subtitle %s of %s: %w", trackID, fileID, err)
	}

	origin, err := service.locate(ctx, playable, trackID)
	if err != nil {
		return WebVTT{}, fmt.Errorf("subtitle %s of %s: %w", trackID, playable.File.ID, err)
	}

	destination, err := service.destination(playable.File.ID, trackID, origin)
	if err != nil {
		return WebVTT{}, fmt.Errorf("subtitle %s of %s: %w", trackID, playable.File.ID, err)
	}

	result, err := service.serve(ctx, origin, destination)
	if err != nil {
		return WebVTT{}, fmt.Errorf("subtitle %s of %s: %w", trackID, playable.File.ID, err)
	}

	return result, nil
}

// serve returns the cached file at destination, converting first on a miss.
func (service *Service) serve(ctx context.Context, origin source, destination string) (WebVTT, error) {
	if cached, err := service.open(ctx, destination); err == nil {
		return cached, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return WebVTT{}, err
	}

	err := service.flights.do(ctx, destination, func(workContext context.Context) error {
		return service.convert(workContext, origin, destination)
	})
	if err != nil {
		return WebVTT{}, err
	}

	return service.open(ctx, destination)
}

func (service *Service) open(ctx context.Context, path string) (WebVTT, error) {
	file, err := service.dependencies.Cache.Open(ctx, path)
	if err != nil {
		return WebVTT{}, fmt.Errorf("open cached subtitle: %w", err)
	}

	return WebVTT(file), nil
}

// convert runs the converter for one destination and trims the cache. A failed trim is only
// logged: the subtitle is converted and servable.
func (service *Service) convert(ctx context.Context, origin source, destination string) error {
	err := service.dependencies.Converter.ConvertWebVTT(ctx, origin.path, origin.track, destination)
	if err != nil {
		return fmt.Errorf("convert: %w", err)
	}

	if err := service.trim(ctx, destination); err != nil {
		service.dependencies.Logger.WarnContext(ctx, "trimming the subtitle cache failed", slog.Any("error", err))
	}

	return nil
}

// locate finds the track and resolves its source below the library root.
func (service *Service) locate(ctx context.Context, playable PlayableFile, trackID string) (source, error) {
	if strings.HasPrefix(trackID, "x:") {
		return service.locateExternal(ctx, playable, trackID)
	}

	return service.locateEmbedded(playable, trackID)
}

func (service *Service) locateEmbedded(playable PlayableFile, trackID string) (source, error) {
	file := playable.File
	ordinal := 0

	for _, stream := range file.Description.Streams {
		if stream.Kind != media.StreamKindSubtitle {
			continue
		}

		if playback.EmbeddedSubtitleTrackID(stream.Index) != trackID {
			ordinal++

			continue
		}

		if stream.IsBitmapSubtitle() || file.Layout.IsDisc() {
			return source{}, fmt.Errorf("track %s is a picture subtitle: %w", trackID, domain.ErrInvalid)
		}

		path, err := service.dependencies.Resolver.Resolve(playable.LibraryRoot, file.RelativePath)
		if err != nil {
			return source{}, fmt.Errorf("resolve media file: %w", err)
		}

		return source{
			path:  path,
			track: Track{Ordinal: ordinal, Codec: stream.Codec},
			stamp: []string{stampOf(file.Size, file.ModifiedAt)},
		}, nil
	}

	return source{}, fmt.Errorf("track %s: %w", trackID, domain.ErrNotFound)
}

func (service *Service) locateExternal(ctx context.Context, playable PlayableFile, trackID string) (source, error) {
	subtitles, err := service.dependencies.External.ExternalSubtitles(ctx, playable.File.ID)
	if err != nil {
		return source{}, fmt.Errorf("list external subtitles: %w", err)
	}

	for _, subtitle := range subtitles {
		if subtitle.TrackID() != trackID {
			continue
		}

		if !subtitle.TextBased {
			return source{}, fmt.Errorf("track %s is a picture subtitle: %w", trackID, domain.ErrInvalid)
		}

		return service.externalSource(ctx, playable, subtitle)
	}

	return source{}, fmt.Errorf("track %s: %w", trackID, domain.ErrNotFound)
}

func (service *Service) externalSource(
	ctx context.Context, playable PlayableFile, subtitle media.ExternalSubtitle,
) (source, error) {
	path, err := service.dependencies.Resolver.Resolve(playable.LibraryRoot, subtitle.RelativePath)
	if err != nil {
		return source{}, fmt.Errorf("resolve subtitle file: %w", err)
	}

	stat, err := service.dependencies.Statter.Stat(ctx, path)
	if err != nil {
		return source{}, fmt.Errorf("stat subtitle file: %w", err)
	}

	return source{
		path:  path,
		track: Track{External: true, Codec: string(subtitle.Format)},
		stamp: []string{stampOf(playable.File.Size, playable.File.ModifiedAt), stampOf(stat.Size, stat.ModifiedAt)},
	}, nil
}

func stampOf(size int64, modifiedAt time.Time) string {
	return strconv.FormatInt(size, 10) + ":" + strconv.FormatInt(modifiedAt.UnixNano(), 10)
}

// destination derives the cache path from the catalog's file ID, a digest of the track ID and
// the source stamp, never from request text: <cache>/subtitles/<fileID>/<track>-<stamp>.vtt.
// It must lie below the cache directory and differ from the source, or the converter would
// overwrite a library file.
func (service *Service) destination(fileID, trackID string, origin source) (string, error) {
	if fileID == "" || fileID == "." || fileID == ".." || strings.ContainsAny(fileID, `/\:`) {
		return "", fmt.Errorf("file id %q is no folder name: %w", fileID, domain.ErrInvalid)
	}

	trackDigest := sha1.Sum([]byte(trackID)) //nolint:gosec // a file name, not a security boundary
	stampDigest := sha256.Sum256([]byte(strings.Join(origin.stamp, "|")))
	name := hex.EncodeToString(trackDigest[:])[:digestLength] + "-" +
		hex.EncodeToString(stampDigest[:])[:digestLength] + cacheExtension

	destination := filepath.Join(service.cacheRoot(), fileID, name)

	relative, err := filepath.Rel(service.cacheRoot(), destination)
	if err != nil || strings.HasPrefix(relative, "..") {
		return "", fmt.Errorf("destination %q leaves the cache: %w", destination, domain.ErrForbidden)
	}

	if strings.EqualFold(filepath.Clean(origin.path), destination) {
		return "", fmt.Errorf("destination equals the source: %w", domain.ErrForbidden)
	}

	return destination, nil
}

func (service *Service) cacheRoot() string {
	return filepath.Join(service.dependencies.CacheDir, cacheDirectoryName)
}
