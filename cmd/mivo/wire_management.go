package main

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"

	"github.com/magpeek/mivo/internal/adapters/fs/folderbrowse"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/adapters/sqlite"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	appscan "github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/app/scantrigger"
)

// scanController adapts the scan service to the library manager's ScanControl port: the
// service's Start returns the scan status and its Cancel waits, while the manager cancels
// first and waits separately.
type scanController struct {
	scans *appscan.Service
}

var _ applibrary.ScanControl = scanController{}

// Start starts a scan of the library unless one runs already.
func (controller scanController) Start(ctx context.Context, libraryID string) error {
	if _, err := controller.scans.Start(ctx, libraryID); err != nil {
		return fmt.Errorf("start scan: %w", err)
	}

	return nil
}

// Cancel asks the running scan to stop and returns at once: the context is already done, so
// the service's wait for the scan to end returns immediately and WaitIdle does the waiting.
func (controller scanController) Cancel(libraryID string) {
	stopWaiting, cancel := context.WithCancel(context.Background())
	cancel()

	// The only error is the done context of the wait, which is intended.
	_ = controller.scans.Cancel(stopWaiting, libraryID)
}

// WaitIdle returns once no scan of the library runs or ctx is done.
func (controller scanController) WaitIdle(ctx context.Context, libraryID string) error {
	if err := controller.scans.WaitIdle(ctx, libraryID); err != nil {
		return fmt.Errorf("wait for scan: %w", err)
	}

	return nil
}

// Forget drops the scan's memory of a deleted library.
func (controller scanController) Forget(libraryID string) {
	controller.scans.Forget(libraryID)
}

// managementAdapters are what library management shares with the rest of the server.
type managementAdapters struct {
	store   *sqlite.ManagedLibraryStore
	scans   *appscan.Service
	trigger *scantrigger.Trigger
}

// management is the wired library management: the use cases of the library endpoints and the
// folder browser, with the installation facts the folder roots answer carries.
type management struct {
	manager *applibrary.Manager
	install restapi.InstallInfo
}

// scanning is the scan service with what depends on it: the triggers that keep the libraries
// current and the library management that stops scans before it changes a library.
type scanning struct {
	scans      *appscan.Service
	trigger    *scantrigger.Trigger
	management management
}

// newScanning builds the scan service, its triggers and the library management and imports
// the configured libraries. The scan service is closed again when a later step fails.
func (app application) newScanning(ctx context.Context, db *sqlite.DB, store *sqlite.ManagedLibraryStore,
	adapters scanAdapters,
) (scanning, error) {
	scans, err := app.newScanService(db, store, adapters)
	if err != nil {
		return scanning{}, err
	}

	trigger, err := app.newScanTrigger(store, scans)
	if err != nil {
		return scanning{}, withCleanupError(ctx, err, closeScansStep, app.closeScans(ctx, scans))
	}

	managing, err := app.newManagement(ctx, managementAdapters{store: store, scans: scans, trigger: trigger})
	if err != nil {
		return scanning{}, withCleanupError(ctx, err, closeScansStep, app.closeScans(ctx, scans))
	}

	return scanning{scans: scans, trigger: trigger, management: managing}, nil
}

// reservedDirectories are the server's own directories no library folder may touch.
func (app application) reservedDirectories() []string {
	candidates := []string{
		app.config.ConfigDir, app.config.Paths.Data, app.config.Paths.Cache, app.config.Paths.Transcode,
	}

	reserved := make([]string, 0, len(candidates))

	for _, directory := range candidates {
		if directory != "" {
			reserved = append(reserved, directory)
		}
	}

	return reserved
}

// newManagement builds the library manager and imports the configured libraries once
// (ADR 0028). The import runs before the listener binds, so no client sees a half-imported
// state, and before the trigger runs, so its first check already lists the imported
// libraries.
func (app application) newManagement(ctx context.Context, adapters managementAdapters) (management, error) {
	reserved := app.reservedDirectories()
	browser := folderbrowse.New(app.config.Browse.Roots, folderbrowse.Options{Reserved: reserved})

	manager, err := applibrary.NewManager(applibrary.ManagerDependencies{
		Catalog:         adapters.store,
		Writer:          adapters.store,
		Ledger:          adapters.store,
		Browser:         newFolderBrowserAdapter(browser, app.config.Browse.Roots),
		Scans:           scanController{scans: adapters.scans},
		Events:          adapters.trigger,
		Suspender:       adapters.trigger,
		Logger:          app.logger,
		AllowedKinds:    enabledLibraryKinds(),
		Reserved:        reserved,
		CaseInsensitive: runtime.GOOS == "windows",
	})
	if err != nil {
		return management{}, fmt.Errorf("create library manager: %w", err)
	}

	if err := app.importConfiguredLibraries(ctx, manager); err != nil {
		return management{}, err
	}

	return management{
		manager: manager,
		install: restapi.InstallInfo{Kind: app.config.Install.Kind, ServiceUser: browser.ServiceUser()},
	}, nil
}

// importConfiguredLibraries turns the libraries of the configuration into managed libraries,
// once per folder: a library the user deleted is not imported again.
func (app application) importConfiguredLibraries(ctx context.Context, manager *applibrary.Manager) error {
	definitions, err := libraryDefinitions(app.config.Libraries)
	if err != nil {
		return err
	}

	result, err := manager.ImportConfigured(ctx, definitions)
	if err != nil {
		return fmt.Errorf("import configured libraries: %w", err)
	}

	app.logger.InfoContext(ctx, "configured libraries imported",
		slog.Int("created", len(result.Created)), slog.Int("skipped", len(result.Skipped)))

	return nil
}
