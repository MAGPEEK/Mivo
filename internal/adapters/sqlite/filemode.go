package sqlite

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"runtime"
)

// databaseFileMode keeps the database and its companion files readable by the server user only.
const databaseFileMode fs.FileMode = 0o600

// windowsGOOS is runtime.GOOS on Windows, where files inherit the data directory ACL instead.
const windowsGOOS = "windows"

// databaseFiles creates the database file and restricts the modes of the files SQLite keeps
// for it. The owner check and chmod are fields so tests can simulate a foreign owner and EPERM.
type databaseFiles struct {
	logger         *slog.Logger
	ownedByProcess func(info fs.FileInfo) bool
	chmod          func(path string, mode fs.FileMode) error
}

func newDatabaseFiles(logger *slog.Logger) databaseFiles {
	return databaseFiles{logger: logger, ownedByProcess: fileOwnedByProcess, chmod: os.Chmod}
}

// companionFileSuffixes returns the suffixes of the database file and the WAL, SHM and
// rollback journal files SQLite keeps next to it.
func companionFileSuffixes() []string {
	return []string{"", "-wal", "-shm", "-journal"}
}

// prepare creates the database file with databaseFileMode before SQLite opens it, because
// SQLite would create it 0644 and the database holds password hashes and session digests
// (ADR 0032). SQLite gives the WAL and SHM files the mode of the database file, so they
// follow. Files left by an older version are tightened to the same mode. On Windows the mode
// bits do not apply: the files inherit the ACL of the data directory.
func (files databaseFiles) prepare(ctx context.Context, databasePath string) error {
	file, err := os.OpenFile(databasePath, os.O_RDWR|os.O_CREATE, databaseFileMode) //nolint:gosec // configured database path in the data directory
	if err != nil {
		return fmt.Errorf("create database file: %w", err)
	}

	err = file.Close()
	if err != nil {
		return fmt.Errorf("close database file: %w", err)
	}

	if runtime.GOOS == windowsGOOS {
		return nil
	}

	for _, suffix := range companionFileSuffixes() {
		err = files.restrict(ctx, databasePath+suffix)
		if err != nil {
			return err
		}
	}

	return nil
}

// restrict removes every permission beyond databaseFileMode from an existing file. A missing
// file is fine: SQLite creates the companion files only when it needs them. A file the
// process does not own, or may not chmod, only gets a warning: a database copied in by hand
// or left behind by a changed container user must still open, as it did before.
func (files databaseFiles) restrict(ctx context.Context, path string) error {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("inspect database file: %w", err)
	}

	if info.Mode().Perm()&^databaseFileMode == 0 {
		return nil
	}

	if !files.ownedByProcess(info) {
		files.warnUnrestricted(ctx, info, "owned by another user")

		return nil
	}

	err = files.chmod(path, databaseFileMode)
	if errors.Is(err, fs.ErrPermission) {
		files.warnUnrestricted(ctx, info, "permission denied")

		return nil
	}

	if err != nil {
		return fmt.Errorf("restrict database file mode: %w", err)
	}

	return nil
}

func (files databaseFiles) warnUnrestricted(ctx context.Context, info fs.FileInfo, reason string) {
	files.logger.WarnContext(ctx, "database file mode left unrestricted",
		slog.String("file", info.Name()),
		slog.String("mode", info.Mode().Perm().String()),
		slog.String("want_mode", databaseFileMode.String()),
		slog.String("reason", reason))
}
