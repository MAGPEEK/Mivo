package sqlite

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDataSourceNameIsAnAbsoluteFileURIThatRoundTripsThePath(t *testing.T) {
	t.Parallel()

	absoluteDirectory := t.TempDir()

	tests := []struct {
		name string
		path string
	}{
		{name: "plain", path: filepath.Join(absoluteDirectory, "server.db")},
		{name: "spaces and URI characters", path: filepath.Join(absoluteDirectory, "Jane Doe #1 %20 ä", "server.db")},
		{name: "relative", path: filepath.Join("data", "server.db")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			dataSource, err := dataSourceName(test.path, writerRole)
			if err != nil {
				t.Fatalf("dataSourceName error = %v", err)
			}

			wantPath, err := filepath.Abs(test.path)
			if err != nil {
				t.Fatalf("Abs error = %v", err)
			}

			if got := pathOfFileURI(t, dataSource); got != wantPath {
				t.Errorf("dataSourceName(%q) = %q decodes to %q, want %q", test.path, dataSource, got, wantPath)
			}

			if strings.Contains(dataSource, `\`) || strings.Contains(dataSource, " ") {
				t.Errorf("dataSourceName(%q) = %q, want forward slashes and no raw spaces", test.path, dataSource)
			}
		})
	}
}

// windowsOS is runtime.GOOS on Windows.
const windowsOS = "windows"

// pathOfFileURI reverses dataSourceName's path encoding: it decodes the URI path and, on
// Windows, drops the slash in front of a drive letter ("/C:/x"); a UNC path ("//server/x")
// keeps both slashes.
func pathOfFileURI(t *testing.T, dataSource string) string {
	t.Helper()

	location, err := url.Parse(dataSource)
	if err != nil {
		t.Fatalf("parse %q: %v", dataSource, err)
	}

	if location.Scheme != fileURIScheme || location.Host != "" {
		t.Fatalf("%q is not a file URI with an empty authority", dataSource)
	}

	path := location.Path
	if runtime.GOOS == windowsOS && !strings.HasPrefix(path, "//") {
		path = strings.TrimPrefix(path, "/")
	}

	return filepath.FromSlash(path)
}

func TestDataSourceNameSetsRoleSpecificParameters(t *testing.T) {
	t.Parallel()

	shared := map[string]string{
		parameterBusyTimeout: "5000",
		parameterForeignKeys: "1",
		parameterJournalMode: "WAL",
		parameterSynchronous: "NORMAL",
	}

	tests := []struct {
		name    string
		role    poolRole
		want    map[string]string
		wantNot string
	}{
		{name: "writer", role: writerRole, want: map[string]string{parameterTxLock: "immediate"}, wantNot: parameterQueryOnly},
		{name: "reader", role: readerRole, want: map[string]string{parameterQueryOnly: "1"}, wantNot: parameterTxLock},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			dataSource, err := dataSourceName("server.db", test.role)
			if err != nil {
				t.Fatalf("dataSourceName error = %v", err)
			}

			location, err := url.Parse(dataSource)
			if err != nil {
				t.Fatalf("parse %q: %v", dataSource, err)
			}

			parameters := location.Query()
			assertParameters(t, parameters, shared)
			assertParameters(t, parameters, test.want)

			if parameters.Has(test.wantNot) {
				t.Errorf("parameter %s is set, want it absent", test.wantNot)
			}
		})
	}
}

func assertParameters(t *testing.T, parameters url.Values, want map[string]string) {
	t.Helper()

	for key, value := range want {
		if got := parameters.Get(key); got != value {
			t.Errorf("parameter %s = %q, want %q", key, got, value)
		}
	}
}

// A data directory on a network share (\\server\share\...) keeps the server in the URI path
// behind an empty authority (file:////server/share/...): SQLite accepts only an empty or
// "localhost" authority, and Windows resolves the path //server/share/... as UNC.
func TestDataSourceNameKeepsAWindowsUNCPath(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != windowsOS {
		t.Skip("UNC paths exist only on Windows")
	}

	const uncPath = `\\nas\media share\Mivo #1\server.db`

	dataSource, err := dataSourceName(uncPath, readerRole)
	if err != nil {
		t.Fatalf("dataSourceName error = %v", err)
	}

	const wantPrefix = "file:////nas/media%20share/Mivo%20%231/server.db?"
	if !strings.HasPrefix(dataSource, wantPrefix) {
		t.Errorf("dataSourceName(%q) = %q, want prefix %q", uncPath, dataSource, wantPrefix)
	}

	if got := pathOfFileURI(t, dataSource); got != uncPath {
		t.Errorf("dataSourceName(%q) = %q decodes to %q, want the UNC path", uncPath, dataSource, got)
	}
}

// The DSN above must also open a real database. The administrative share of the local
// machine (\\localhost\C$) reaches the temporary directory over UNC; the test is skipped
// where that share is disabled or not accessible.
func TestOpenWorksThroughAWindowsUNCPath(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != windowsOS {
		t.Skip("UNC paths exist only on Windows")
	}

	directory := localAdministrativeSharePath(t, t.TempDir())

	db, err := Open(t.Context(), Options{Path: filepath.Join(directory, "server.db")})
	if err != nil {
		t.Fatalf("Open through %s error = %v", directory, err)
	}

	if err := errors.Join(db.Migrate(t.Context()), db.Ping(t.Context()), db.Close()); err != nil {
		t.Errorf("Migrate, Ping and Close through %s error = %v", directory, err)
	}
}

// localAdministrativeSharePath maps C:\dir to \\localhost\C$\dir and skips the test when the
// result is not reachable.
func localAdministrativeSharePath(t *testing.T, localPath string) string {
	t.Helper()

	volume := filepath.VolumeName(localPath)
	if len(volume) != len("C:") {
		t.Skipf("temporary directory %s is not on a drive letter", localPath)
	}

	sharePath := `\\localhost\` + volume[:1] + `$` + strings.TrimPrefix(localPath, volume)
	if _, err := os.Stat(sharePath); err != nil {
		t.Skipf("administrative share not reachable: %v", err)
	}

	return sharePath
}
