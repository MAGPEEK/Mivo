package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/adapters/fs/libraryfs"
	"github.com/magpeek/mivo/internal/adapters/fs/transcodedir"
	"github.com/magpeek/mivo/internal/adapters/http/stream"
	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/adapters/uuidgen"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain/playback"
)

const (
	// maxTranscodeHeight caps transcoded video at 1080 lines (OD-5 B); the configuration
	// has no key for it in milestone 2.
	maxTranscodeHeight = 1080

	// playbackCloseTimeout bounds stopping every session at shutdown (part of
	// shutdownBudget). The stops run in parallel and a killed FFmpeg normally exits within
	// milliseconds; when the bound runs out, the cancelled service context kills what is left
	// and the next start purges leftover directories.
	playbackCloseTimeout = 5 * time.Second

	// purgeTimeout bounds deleting the session directories of the previous run.
	purgeTimeout = time.Minute

	// segmentTemplateMarker starts an FFmpeg segment template: the hls muxer would expand
	// it in the transcode path (ffmpeg.HLSArguments rejects such a directory).
	segmentTemplateMarker = "%"

	// closePlaybackStep names the shutdown step in the error when sessions do not stop in
	// time.
	closePlaybackStep = "close playback service"
)

// playbackAdapters are the adapters the playback use case and the stream handlers share.
type playbackAdapters struct {
	files     *libraryfs.FileSystem
	prober    *ffmpeg.Prober
	workspace *transcodedir.Workspace
	tools     externalTools
}

// newWorkspace manages <transcode>/sessions. A transcode path with "%" is refused here,
// because FFmpeg would read it as a segment template and every HLS session would fail.
func (app application) newWorkspace() (*transcodedir.Workspace, error) {
	path := app.config.Paths.Transcode
	if strings.Contains(path, segmentTemplateMarker) {
		return nil, fmt.Errorf("config: paths.transcode %q contains %q, which FFmpeg reads as a segment template; choose a directory without it",
			path, segmentTemplateMarker)
	}

	workspace, err := transcodedir.New(path)
	if err != nil {
		return nil, fmt.Errorf("create transcode workspace: %w", err)
	}

	return workspace, nil
}

// newPlaybackService builds the playback use case and purges the session directories a
// previous run left behind, before the HTTP server accepts the first session (ADR 0024).
func (app application) newPlaybackService(ctx context.Context, db *sqlite.DB, adapters playbackAdapters) (*appplayback.Service, error) {
	producer, err := ffmpeg.NewProducer(ffmpeg.ProducerOptions{
		FFmpegPath: adapters.tools.ffmpeg, Inputs: adapters.files, Logger: app.logger,
	})
	if err != nil {
		return nil, fmt.Errorf("create stream producer: %w", err)
	}

	service, err := appplayback.NewService(appplayback.Dependencies{
		Files: sqlite.NewCatalogReader(db), Paths: adapters.files, Keyframes: adapters.prober,
		Producer: producer, Workspace: adapters.workspace, URLs: stream.NewURLBuilder(),
		IDs: uuidgen.New(), Clock: time.Now, After: time.After, Logger: app.logger,
	}, appplayback.Options{
		Capabilities:         adapters.tools.capabilities,
		Policy:               playback.Policy{MaxTranscodeHeight: maxTranscodeHeight},
		MaxTranscodeSessions: app.config.Transcode.MaxSessions,
	})
	if err != nil {
		return nil, fmt.Errorf("create playback service: %w", err)
	}

	if err := app.purgeWorkspace(ctx, service); err != nil {
		return nil, withCleanupError(ctx, err, closePlaybackStep, app.closePlayback(ctx, service))
	}

	return service, nil
}

// purgeWorkspace deletes the leftovers of the previous run. A sessions entry that is no
// plain directory makes the workspace unusable and fails the start; a leftover that cannot
// be deleted only costs disk space and is logged.
func (app application) purgeWorkspace(ctx context.Context, service *appplayback.Service) error {
	purgeContext, cancel := context.WithTimeout(ctx, purgeTimeout)
	defer cancel()

	err := service.PurgeWorkspace(purgeContext)

	switch {
	case err == nil:
		return nil
	case errors.Is(err, transcodedir.ErrSessionsNotPlainDirectory):
		return fmt.Errorf("transcode workspace %s: the sessions entry must be a plain directory, not a symlink, junction or file; remove it: %w",
			app.config.Paths.Transcode, err)
	case ctx.Err() != nil:
		return fmt.Errorf("purge transcode workspace: %w", err)
	default:
		app.logger.WarnContext(ctx, "leftover playback session directories could not be deleted", slog.Any("error", err))

		return nil
	}
}

// closePlayback stops every session (FFmpeg killed, directories removed) and the reaper,
// bounded by playbackCloseTimeout. It runs detached from ctx, which is done at shutdown.
func (app application) closePlayback(ctx context.Context, service *appplayback.Service) error {
	closeContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), playbackCloseTimeout)
	defer cancel()

	if err := service.Close(closeContext); err != nil {
		return fmt.Errorf("stop playback sessions: %w", err)
	}

	return nil
}

// newStreamHandlers serves the files of playback sessions on the API mux (ADR 0024).
func (app application) newStreamHandlers(service *appplayback.Service, adapters playbackAdapters) (*stream.Handlers, error) {
	handlers, err := stream.New(stream.Options{
		Sessions: service, Library: adapters.files, Output: adapters.workspace, Logger: app.logger,
	})
	if err != nil {
		return nil, fmt.Errorf("create stream handlers: %w", err)
	}

	return handlers, nil
}
