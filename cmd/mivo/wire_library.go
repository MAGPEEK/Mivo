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

// newLibraryService builds the catalog use cases and syncs the configured libraries into the
// catalog before any client can ask for them (ADR 0022).
func (app application) newLibraryService(ctx context.Context, db *sqlite.DB, files *libraryfs.FileSystem) (*applibrary.Service, error) {
	definitions, err := libraryDefinitions(app.config.Libraries)
	if err != nil {
		return nil, err
	}

	service, err := applibrary.NewService(applibrary.Dependencies{
		Libraries: sqlite.NewLibraryRepository(db, time.Now),
		Items:     sqlite.NewCatalogReader(db),
		IDs:       uuidgen.New(),
		Roots:     files,
		Logger:    app.logger,
	})
	if err != nil {
		return nil, fmt.Errorf("create library service: %w", err)
	}

	libraries, err := service.SyncLibraries(ctx, definitions)
	if err != nil {
		return nil, fmt.Errorf("sync configured libraries: %w", err)
	}

	app.logger.InfoContext(ctx, "libraries synced from the configuration", slog.Int("libraries", len(libraries)))

	return service, nil
}

// newScanService builds the scan use case on the real walker, resolver and ffprobe.
func (app application) newScanService(db *sqlite.DB, files *libraryfs.FileSystem, prober *ffmpeg.Prober) (*appscan.Service, error) {
	service, err := appscan.NewService(appscan.Dependencies{
		Libraries: sqlite.NewLibraryRepository(db, time.Now),
		Walker:    files,
		Resolver:  files,
		Prober:    prober,
		Catalog:   sqlite.NewCatalogWriter(db),
		IDs:       uuidgen.New(),
		Clock:     time.Now,
		Logger:    app.logger,
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
