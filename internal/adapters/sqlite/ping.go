package sqlite

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// pingQuery reads the schema table, so it starts a read transaction on the database file.
// The driver's own Ping runs "SELECT 1", which SQLite answers without touching the file and
// which would report a database whose file became unreadable (a vanished network share, a
// lock held by another process) as healthy.
const pingQuery = "SELECT COUNT(*) FROM sqlite_schema"

// Ping reports whether the database answers a query that reads its file. It implements the
// app/system DatabaseChecker port and returns when ctx is done.
//
// It uses the reader pool only: the writer has a single connection, and a long write
// transaction holding it would make a healthy database look unreachable.
//
// The port contract leaves logging to the adapter, because Health shows clients only a fixed
// message. A cancelled ctx means the caller went away, not that the database failed, so it is
// logged at debug level; every other failure, including a timeout, at warn level.
func (db *DB) Ping(ctx context.Context) error {
	var schemaEntries int

	err := db.reader.QueryRowContext(ctx, pingQuery).Scan(&schemaEntries)
	if err == nil {
		return nil
	}

	level := slog.LevelWarn
	if errors.Is(err, context.Canceled) {
		level = slog.LevelDebug
	}

	db.logger.LogAttrs(ctx, level, "database ping failed", slog.Any("error", err))

	return fmt.Errorf("ping database: %w", err)
}
