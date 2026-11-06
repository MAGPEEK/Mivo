package sqlite

import (
	"bytes"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// openFileMode is wider than databaseFileMode on every platform: Windows reports 0666 for a
// writable file whatever mode was requested.
const openFileMode fs.FileMode = 0o644

var errChmodFailed = errors.New("chmod failed")

func TestRestrictFileModeHandlesOwnerAndChmodOutcomes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		owned     bool
		chmodErr  error
		wantChmod bool
		wantWarn  string
		wantErr   error
	}{
		{name: "owned file is tightened", owned: true, wantChmod: true},
		{name: "foreign owner only warns", owned: false, wantWarn: "owned by another user"},
		{
			name: "permission denied only warns", owned: true, wantChmod: true,
			chmodErr: &fs.PathError{Op: "chmod", Path: "server.db", Err: fs.ErrPermission},
			wantWarn: "permission denied",
		},
		{name: "other chmod error fails", owned: true, wantChmod: true, chmodErr: errChmodFailed, wantErr: errChmodFailed},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "server.db")
			if err := os.WriteFile(path, nil, openFileMode); err != nil {
				t.Fatalf("create %s: %v", path, err)
			}

			var output bytes.Buffer
			chmodCalls := 0
			files := databaseFiles{
				logger:         slog.New(slog.NewTextHandler(&output, nil)),
				ownedByProcess: func(fs.FileInfo) bool { return test.owned },
				chmod: func(string, fs.FileMode) error {
					chmodCalls++

					return test.chmodErr
				},
			}

			err := files.restrict(t.Context(), path)
			assertRestrictError(t, err, test.wantErr)

			if gotChmod := chmodCalls > 0; gotChmod != test.wantChmod {
				t.Errorf("chmod called = %v, want %v", gotChmod, test.wantChmod)
			}

			assertWarning(t, output.String(), test.wantWarn)
		})
	}
}

func TestRestrictFileModeIgnoresMissingFiles(t *testing.T) {
	t.Parallel()

	files := newDatabaseFiles(slog.New(slog.DiscardHandler))
	if err := files.restrict(t.Context(), filepath.Join(t.TempDir(), "server.db-wal")); err != nil {
		t.Errorf("restrict missing file error = %v, want nil", err)
	}
}

func assertRestrictError(t *testing.T, err, wantErr error) {
	t.Helper()

	if (wantErr == nil && err != nil) || (wantErr != nil && !errors.Is(err, wantErr)) {
		t.Errorf("restrict error = %v, want %v", err, wantErr)
	}
}

func assertWarning(t *testing.T, output, wantReason string) {
	t.Helper()

	if wantReason == "" {
		if output != "" {
			t.Errorf("log output = %q, want none", output)
		}

		return
	}

	if !strings.Contains(output, "level=WARN") || !strings.Contains(output, wantReason) {
		t.Errorf("log output = %q, want a warning with reason %q", output, wantReason)
	}
}
