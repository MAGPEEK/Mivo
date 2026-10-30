package sqlite

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// Ping reports whether the database answers a query. It implements the app/system
// DatabaseChecker port and returns when ctx is done.
//
// It pings the reader pool only: the writer has a single connection, and a long write
// transaction holding it would make a healthy database look unreachable.
//
// The port contract leaves logging to the adapter, because Health shows clients only a fixed
// message. A cancelled ctx means the caller went away, not that the database failed, so it is
// logged at debug level; every other failure, including a timeout, at warn level.
func (db *DB) Ping(ctx context.Context) error {
	err := db.reader.PingContext(ctx)
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
