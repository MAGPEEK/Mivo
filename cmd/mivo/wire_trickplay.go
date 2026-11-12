package main

import (
	"context"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/adapters/fs/cachedir"
	"github.com/magpeek/mivo/internal/adapters/sqlite"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/app/trickplay"
)

// trickplayGenerator adapts the FFmpeg generator to trickplay.Generator.
type trickplayGenerator struct {
	generator *ffmpeg.TrickplayGenerator
}

// GenerateTrickplay implements trickplay.Generator.
func (adapter trickplayGenerator) GenerateTrickplay(ctx context.Context, source, destinationDir string) (trickplay.Manifest, error) {
	manifest, err := adapter.generator.GenerateTrickplay(ctx, source, destinationDir, ffmpeg.TrickplayOptions{})
	if err != nil {
		return trickplay.Manifest{}, fmt.Errorf("generate trickplay: %w", err)
	}

	return trickplay.Manifest{
		TileWidth: manifest.TileWidth, TileHeight: manifest.TileHeight, Columns: manifest.Columns,
		Rows: manifest.Rows, Interval: manifest.Interval, SheetCount: manifest.SheetCount,
	}, nil
}

// systemClock is the wall clock of the trickplay worker.
type systemClock struct{}

// Now implements trickplay.Clock.
func (systemClock) Now() time.Time { return time.Now() }

// After implements trickplay.Clock.
func (systemClock) After(wait time.Duration) <-chan time.Time { return time.After(wait) }

// newTrickplayService builds the sprite sheet service. Without a usable FFmpeg it has no
// generator and answers "unavailable" to requests. Start runs its worker.
func (app application) newTrickplayService(db *sqlite.DB, adapters playbackAdapters, sessions *appplayback.Service) (*trickplay.Service, error) {
	cache, err := cachedir.NewTrickplay(app.config.Paths.Cache)
	if err != nil {
		return nil, fmt.Errorf("create trickplay cache: %w", err)
	}

	var generator trickplay.Generator

	if adapters.tools.ffmpegUsable {
		built, err := ffmpeg.NewTrickplayGenerator(ffmpeg.TrickplayGeneratorOptions{
			FFmpegPath: adapters.tools.ffmpeg, Prober: adapters.prober, Logger: app.logger,
			LowPriority: lowPriority(app.config.Transcode),
		})
		if err != nil {
			return nil, fmt.Errorf("create trickplay generator: %w", err)
		}

		generator = trickplayGenerator{generator: built}
	}

	service, err := trickplay.New(trickplay.Options{
		Files: sqlite.NewCatalogReader(db), Paths: adapters.files, Generator: generator,
		Store: cache, Cache: cache, Sessions: adminSessions{sessions: sessions},
		Clock: systemClock{}, Logger: app.logger, MaxSessions: app.config.Transcode.MaxSessions,
	})
	if err != nil {
		return nil, fmt.Errorf("create trickplay service: %w", err)
	}

	return service, nil
}
