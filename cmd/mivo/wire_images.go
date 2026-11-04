package main

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/adapters/fs/imagecache"
	"github.com/magpeek/mivo/internal/adapters/fs/libraryfs"
	imagehttp "github.com/magpeek/mivo/internal/adapters/http/images"
	"github.com/magpeek/mivo/internal/adapters/sqlite"
	appimages "github.com/magpeek/mivo/internal/app/images"
)

const (
	// imageCacheDirectory is the directory of rendered pictures below paths.cache.
	imageCacheDirectory = "images"

	// imagePurgeTimeout bounds one purge of the image cache; it only unlinks files, so the
	// bound guards against a hung disk, not against work.
	imagePurgeTimeout = time.Minute
)

// imageServices are the use case behind the image endpoint and its HTTP handler.
type imageServices struct {
	service  *appimages.Service
	handlers *imagehttp.Handlers
}

// newImageServices builds the image endpoint on the sqlite artwork source, the library
// resolver, the FFmpeg renderer and the disk cache at <paths.cache>/images.
func (app application) newImageServices(db *sqlite.DB, files *libraryfs.FileSystem, tools externalTools) (imageServices, error) {
	cache, err := imagecache.New(filepath.Join(app.config.Paths.Cache, imageCacheDirectory), app.logger)
	if err != nil {
		return imageServices{}, fmt.Errorf("create image cache: %w", err)
	}

	renderer, err := ffmpeg.NewImageRenderer(ffmpeg.ImageRendererOptions{FFmpegPath: tools.ffmpeg, Logger: app.logger})
	if err != nil {
		return imageServices{}, fmt.Errorf("create image renderer: %w", err)
	}

	service := appimages.NewService(appimages.Dependencies{
		Source: sqlite.NewCatalogReader(db), Resolver: files, Renderer: renderer, Cache: cache, Logger: app.logger,
	})

	handlers, err := imagehttp.New(imagehttp.Options{Images: service, Logger: app.logger})
	if err != nil {
		return imageServices{}, fmt.Errorf("create image handlers: %w", err)
	}

	return imageServices{service: service, handlers: handlers}, nil
}

// purgeImageCache drops renderings of artwork that is gone or changed. A failure only costs
// disk space, so it is logged and never stops the caller.
func (app application) purgeImageCache(ctx context.Context, service *appimages.Service) {
	purgeContext, cancel := context.WithTimeout(ctx, imagePurgeTimeout)
	defer cancel()

	if err := service.Purge(purgeContext); err != nil {
		app.logger.WarnContext(ctx, "image cache purge failed", slog.Any("error", err))
	}
}

// purgeImageCacheInBackground purges once without delaying the listener. The returned stop
// function cancels the purge and waits until it has ended, so the shutdown stays bounded.
func (app application) purgeImageCacheInBackground(ctx context.Context, service *appimages.Service) (stop func()) {
	purgeContext, cancel := context.WithCancel(ctx)

	var group sync.WaitGroup

	group.Go(func() { app.purgeImageCache(purgeContext, service) })

	return func() {
		cancel()
		group.Wait()
	}
}
