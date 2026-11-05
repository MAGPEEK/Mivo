package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/adapters/fs/libraryfs"
	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/adapters/uuidgen"
	appimages "github.com/magpeek/mivo/internal/app/images"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	appscan "github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/platform/config"
)

// scanCloseTimeout bounds waiting for running scans at shutdown (part of shutdownBudget).
// Close cancels them, which kills their ffprobe processes, so they end within moments; the
// bound only keeps a hung network mount from blocking the exit.
const scanCloseTimeout = 2 * time.Second

// closeScansStep names the shutdown step in the error when the scans do not end in time.
const closeScansStep = "close scan service"

// libraryDefinitions converts the configured libraries into domain definitions (platform
// must not import domain) and validates them with the domain's rules, which the config
// package cannot check, such as the name length. An error names the entry.
func libraryDefinitions(libraries []config.LibraryConfig) ([]library.Definition, error) {
	definitions := make([]library.Definition, 0, len(libraries))

	for index, configured := range libraries {
		definition, err := libraryDefinition(configured)
		if err != nil {
			return nil, fmt.Errorf("config: libraries[%d] (%s): %w", index, configured.Path, err)
		}

		definitions = append(definitions, definition)
	}

	return definitions, nil
}

func libraryDefinition(configured config.LibraryConfig) (library.Definition, error) {
	kind, err := library.ParseKind(configured.Kind)
	if err != nil {
		return library.Definition{}, fmt.Errorf("convert library: %w", err)
	}

	definition := library.Definition{Name: configured.Name, Root: configured.Path, Kind: kind}
	if err := definition.Validate(); err != nil {
		return library.Definition{}, fmt.Errorf("convert library: %w", err)
	}

	return definition, nil
}

// newLibraryStore is the one library store of the server: it writes the libraries, keeps the
// import ledger and is the library source of the scan and its triggers (ADR 0028).
func newLibraryStore(db *sqlite.DB) *sqlite.ManagedLibraryStore {
	return sqlite.NewManagedLibraryStore(db, time.Now, uuidgen.New().NewID)
}

// newLibraryServices builds the item use cases and the library reader of the API responses.
func newLibraryServices(db *sqlite.DB, store *sqlite.ManagedLibraryStore, files *libraryfs.FileSystem,
) (*applibrary.Service, *applibrary.ManagedCatalog, error) {
	reader := sqlite.NewCatalogReader(db)

	service, err := applibrary.NewService(applibrary.Dependencies{
		Libraries: store, Items: reader, Children: reader, Listings: reader,
		FolderItems: reader, Directories: files, Roots: files,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create library service: %w", err)
	}

	managed, err := newManagedCatalog(store, files)
	if err != nil {
		return nil, nil, err
	}

	return service, managed, nil
}

// newManagedCatalog reads the libraries with folders, options and item counts.
func newManagedCatalog(store *sqlite.ManagedLibraryStore, files *libraryfs.FileSystem) (*applibrary.ManagedCatalog, error) {
	catalog, err := applibrary.NewManagedCatalog(applibrary.ManagedCatalogDependencies{
		Libraries: store, Counts: store, Roots: files,
	})
	if err != nil {
		return nil, fmt.Errorf("create managed library catalog: %w", err)
	}

	return catalog, nil
}

// scanAdapters are the adapters the scan service shares with the rest of the server.
type scanAdapters struct {
	files  *libraryfs.FileSystem
	prober *ffmpeg.Prober
	images *appimages.Service
}

// newScanService builds the v2 scan use case on the real walker, resolver and ffprobe, the
// sqlite catalog, and the metadata refresher that reads NFO files and picks artwork. A
// finished scan purges the image cache, whose renderings the scan may have outdated.
func (app application) newScanService(db *sqlite.DB, store *sqlite.ManagedLibraryStore, adapters scanAdapters) (*appscan.Service, error) {
	catalog := sqlite.NewLibraryCatalog(db, time.Now)

	refresher, err := appscan.NewMetadataRefresher(appscan.MetadataRefresherDeps{
		Catalog: catalog, Reader: adapters.files, Parser: nfoParser{}, Logger: app.logger,
	})
	if err != nil {
		return nil, fmt.Errorf("create metadata refresher: %w", err)
	}

	service, err := appscan.NewService(appscan.Dependencies{
		Libraries: store,
		Walker:    adapters.files,
		Resolver:  adapters.files,
		Prober:    adapters.prober,
		CatalogV2: catalog,
		Refresher: refresher,
		Finished: func(ctx context.Context, _ string) {
			app.purgeImageCache(ctx, adapters.images)
		},
		IDs:    uuidgen.New(),
		Clock:  time.Now,
		Logger: app.logger,
	})
	if err != nil {
		return nil, fmt.Errorf("create scan service: %w", err)
	}

	return service, nil
}

// startLibraryScans scans every library once at startup (OD-3), so files added while the
// server was down appear without a click. A failure is logged, not fatal: the scan button
// still works.
func (app application) startLibraryScans(ctx context.Context, scans *appscan.Service) {
	if err := scans.StartAll(ctx); err != nil {
		app.logger.WarnContext(ctx, "startup library scan not started", slog.Any("error", err))
	}
}

// closeScans cancels the running scans and waits for them, bounded by scanCloseTimeout. It
// runs detached from ctx, which is already done at shutdown.
func (app application) closeScans(ctx context.Context, scans *appscan.Service) error {
	closeContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), scanCloseTimeout)
	defer cancel()

	if err := scans.Close(closeContext); err != nil {
		return fmt.Errorf("wait for library scans: %w", err)
	}

	app.logger.InfoContext(closeContext, "library scans stopped")

	return nil
}
