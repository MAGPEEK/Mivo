package sqlite

import (
	"net/url"
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

// pathOfFileURI reverses dataSourceName's path encoding: it decodes the URI path and, on
// Windows, drops the slash in front of the drive letter.
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
	if runtime.GOOS == "windows" {
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
		dataSource, err := dataSourceName("server.db", test.role)
		if err != nil {
			t.Fatalf("%s: dataSourceName error = %v", test.name, err)
		}

		location, err := url.Parse(dataSource)
		if err != nil {
			t.Fatalf("%s: parse %q: %v", test.name, dataSource, err)
		}

		parameters := location.Query()
		assertParameters(t, test.name, parameters, shared)
		assertParameters(t, test.name, parameters, test.want)

		if parameters.Has(test.wantNot) {
			t.Errorf("%s: parameter %s is set, want it absent", test.name, test.wantNot)
		}
	}
}

func assertParameters(t *testing.T, role string, parameters url.Values, want map[string]string) {
	t.Helper()

	for key, value := range want {
		if got := parameters.Get(key); got != value {
			t.Errorf("%s: parameter %s = %q, want %q", role, key, got, value)
		}
	}
}
