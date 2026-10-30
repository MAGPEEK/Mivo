package sqlite

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/pressly/goose/v3"
)

// migrationsDirectory holds the goose SQL migrations. sqlc reads the same files as schema.
const migrationsDirectory = "migrations"

// embeddedMigrations is package-level because go:embed only works on package-level
// variables. The FS is read-only, so it is no shared mutable state.
//
//go:embed migrations/*.sql
var embeddedMigrations embed.FS

// Migrate applies all pending embedded migrations through the writer pool. It is
// idempotent: when the schema is current it changes nothing.
//
// It uses goose's Provider API, which keeps all state in the provider value; the global
// registry of Go migrations is disabled so no package-level goose state is involved.
func (db *DB) Migrate(ctx context.Context) error {
	provider, err := db.newMigrationProvider()
	if err != nil {
		return err
	}

	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("apply database migrations: %w", err)
	}

	for _, result := range results {
		db.logger.LogAttrs(ctx, slog.LevelInfo, "applied database migration",
			slog.Int64("version", result.Source.Version),
			slog.Duration("duration", result.Duration))
	}

	return nil
}

func (db *DB) newMigrationProvider() (*goose.Provider, error) {
	migrations, err := fs.Sub(embeddedMigrations, migrationsDirectory)
	if err != nil {
		return nil, fmt.Errorf("open embedded migrations: %w", err)
	}

	// goose falls back to the standard library logger, which writes to stderr, when no logger
	// is given. Passing the injected logger keeps every record in the server's log.
	provider, err := goose.NewProvider(goose.DialectSQLite3, db.writer, migrations,
		goose.WithDisableGlobalRegistry(true),
		goose.WithSlog(db.logger),
	)
	if err != nil {
		return nil, fmt.Errorf("create migration provider: %w", err)
	}

	return provider, nil
}
