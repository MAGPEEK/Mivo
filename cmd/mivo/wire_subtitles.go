package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"time"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/adapters/fs/cachedir"
	"github.com/magpeek/mivo/internal/adapters/fs/libraryfs"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/adapters/sqlite"
	appaccess "github.com/magpeek/mivo/internal/app/access"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	appsubtitles "github.com/magpeek/mivo/internal/app/subtitles"
	"github.com/magpeek/mivo/internal/domain"
	domainaccess "github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

const (
	// subtitleConversionTimeout bounds one conversion in the shim, on top of the extractor's
	// own limit: a hung converter must not hold one of the service's two slots forever (M4-13c).
	subtitleConversionTimeout = 45 * time.Second

	// subtitleSweepTimeout bounds the startup sweep of the subtitle cache.
	subtitleSweepTimeout = time.Minute
)

// webVTTExtractor is what the converter shim needs of ffmpeg.SubtitleExtractor.
type webVTTExtractor interface {
	ExtractWebVTT(ctx context.Context, source string, track ffmpeg.SubtitleTrack, destination string) error
}

// subtitleConverter adapts the FFmpeg extractor to subtitles.Converter and bounds each run.
type subtitleConverter struct {
	extractor webVTTExtractor
	timeout   time.Duration
	// noWebVTTEncoder is set when FFmpeg answered the capability query without a webvtt encoder:
	// every conversion would fail inside FFmpeg, so the service reports the server as unable
	// (domain.ErrUnavailable, 503) instead of the track as unsupported (422).
	noWebVTTEncoder bool
}

// ConvertWebVTT implements subtitles.Converter.
func (converter subtitleConverter) ConvertWebVTT(ctx context.Context, source string, track appsubtitles.Track, destination string) error {
	if converter.noWebVTTEncoder {
		return fmt.Errorf("convert subtitle: the bundled FFmpeg has no webvtt encoder: %w", domain.ErrUnavailable)
	}

	runContext, cancel := context.WithTimeout(ctx, converter.timeout)
	defer cancel()

	err := converter.extractor.ExtractWebVTT(runContext, source,
		ffmpeg.SubtitleTrack{External: track.External, Ordinal: track.Ordinal, Codec: track.Codec}, destination)
	if err != nil {
		return fmt.Errorf("convert subtitle: %w", err)
	}

	return nil
}

// subtitleFiles adapts the catalog reader to subtitles.FileSource: the type of a playable file
// differs between the packages, the lookup is the same.
type subtitleFiles struct {
	reader *sqlite.CatalogReader
}

// PlayableFile implements subtitles.FileSource.
func (files subtitleFiles) PlayableFile(ctx context.Context, fileID string, restriction domainaccess.Restriction) (appsubtitles.PlayableFile, error) {
	playable, err := files.reader.PlayableFile(ctx, fileID, restriction)
	if err != nil {
		return appsubtitles.PlayableFile{}, fmt.Errorf("playable file for subtitles: %w", err)
	}

	return appsubtitles.PlayableFile{File: playable.File, LibraryRoot: playable.LibraryRoot}, nil
}

// FileKnown implements subtitles.FileSource.
func (files subtitleFiles) FileKnown(ctx context.Context, fileID string) (bool, error) {
	known, err := files.reader.FileKnown(ctx, fileID)
	if err != nil {
		return false, fmt.Errorf("file known: %w", err)
	}

	return known, nil
}

// externalSubtitleLookup reads the stored subtitle files of one media file. The callers have
// checked the file's visibility already.
type externalSubtitleLookup struct {
	reader *sqlite.CatalogReader
}

// ExternalSubtitles implements subtitles.ExternalSubtitles.
func (lookup externalSubtitleLookup) ExternalSubtitles(ctx context.Context, fileID string) ([]media.ExternalSubtitle, error) {
	subtitles, err := lookup.reader.ExternalSubtitlesOfFile(ctx, fileID)
	if err != nil {
		return nil, fmt.Errorf("external subtitles: %w", err)
	}

	return subtitles, nil
}

// Tracks implements playback.SubtitleTracks and restapi.SubtitleTrackLister: the stored files
// as tracks of the media file.
func (lookup externalSubtitleLookup) Tracks(ctx context.Context, fileID string) ([]playback.SubtitleTrack, error) {
	subtitles, err := lookup.ExternalSubtitles(ctx, fileID)
	if err != nil {
		return nil, err
	}

	return externalTracks(subtitles), nil
}

// externalTracks lists stored subtitle files as the tracks the decision and the media file
// resource know.
func externalTracks(subtitles []media.ExternalSubtitle) []playback.SubtitleTrack {
	tracks := make([]playback.SubtitleTrack, 0, len(subtitles))

	for _, subtitle := range subtitles {
		tracks = append(tracks, playback.SubtitleTrack{
			ID: subtitle.TrackID(), Language: subtitle.Language, Title: subtitle.Title, Format: string(subtitle.Format),
			Forced: subtitle.Forced, SDH: subtitle.SDH, Default: subtitle.Default, External: true, TextBased: subtitle.TextBased,
		})
	}

	return tracks
}

// fileStatter stats a resolved subtitle file. The path comes from libraryfs.Resolve, which
// confined it to the library root.
type fileStatter struct{}

// Stat implements subtitles.Statter.
func (fileStatter) Stat(ctx context.Context, path string) (appsubtitles.FileStat, error) {
	if err := ctx.Err(); err != nil {
		return appsubtitles.FileStat{}, fmt.Errorf("stat subtitle: %w", err)
	}

	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return appsubtitles.FileStat{}, fmt.Errorf("subtitle file: %w", domain.ErrNotFound)
	}

	if err != nil {
		return appsubtitles.FileStat{}, fmt.Errorf("stat subtitle file: %w", err)
	}

	return appsubtitles.FileStat{Size: info.Size(), ModifiedAt: info.ModTime()}, nil
}

// sessionItems names the catalog item of a session's file for the session list.
type sessionItems struct {
	reader *sqlite.CatalogReader
}

// ItemOfFile implements restapi.SessionItemLookup.
func (items sessionItems) ItemOfFile(ctx context.Context, fileID string) (restapi.SessionItem, error) {
	item, err := items.reader.ItemOfFile(ctx, fileID)
	if err != nil {
		return restapi.SessionItem{}, fmt.Errorf("item of file: %w", err)
	}

	return restapi.SessionItem{ID: item.ID, Title: item.Title}, nil
}

// newSubtitleService builds the WebVTT service and sweeps its cache. A failed sweep only
// leaves old files behind, so it is logged.
func (app application) newSubtitleService(ctx context.Context, db *sqlite.DB, files *libraryfs.FileSystem, tools externalTools) (*appsubtitles.Service, error) {
	extractor, err := ffmpeg.NewSubtitleExtractor(ffmpeg.SubtitleExtractorOptions{
		FFmpegPath: tools.ffmpeg, Logger: app.logger, LowPriority: lowPriority(app.config.Transcode),
	})
	if err != nil {
		return nil, fmt.Errorf("create subtitle extractor: %w", err)
	}

	cache, err := cachedir.NewSubtitles(app.config.Paths.Cache)
	if err != nil {
		return nil, fmt.Errorf("create subtitle cache: %w", err)
	}

	reader := sqlite.NewCatalogReader(db)

	service, err := appsubtitles.NewService(appsubtitles.Dependencies{
		Files: subtitleFiles{reader: reader}, External: externalSubtitleLookup{reader: reader},
		Resolver: files, Statter: fileStatter{},
		Converter: subtitleConverter{
			extractor: extractor, timeout: subtitleConversionTimeout,
			noWebVTTEncoder: tools.ffmpegUsable && tools.capabilities.WebVTTEncoder == "",
		},
		Cache: cache, CacheDir: app.config.Paths.Cache, Logger: app.logger,
	})
	if err != nil {
		return nil, fmt.Errorf("create subtitle service: %w", err)
	}

	sweepContext, cancel := context.WithTimeout(ctx, subtitleSweepTimeout)
	defer cancel()

	if err := service.Sweep(sweepContext); err != nil {
		app.logger.WarnContext(ctx, "subtitle cache sweep failed", slog.Any("error", err))
	}

	return service, nil
}

// systemViewer is the viewer of work the server does for itself, such as the trickplay worker
// asking which re-encodes run: an administrator over everything.
func systemViewer() domainaccess.Viewer {
	return domainaccess.Viewer{
		Admin: true, Libraries: domainaccess.AllLibraries(), Limit: domainaccess.NoLimit(),
	}
}

// sessionLister is the viewer-scoped session list of the playback service.
type sessionLister interface {
	List(ctx context.Context) ([]appplayback.SessionInfo, error)
}

// adminSessions lists every playback session for the trickplay worker, which has no viewer.
type adminSessions struct {
	sessions sessionLister
}

// List implements trickplay.SessionLister.
func (admin adminSessions) List(ctx context.Context) ([]appplayback.SessionInfo, error) {
	infos, err := admin.sessions.List(appaccess.WithViewer(ctx, systemViewer()))
	if err != nil {
		return nil, fmt.Errorf("list sessions as the system: %w", err)
	}

	return infos, nil
}
