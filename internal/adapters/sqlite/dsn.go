package sqlite

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// Connection settings from ADR 0004. They are DSN parameters of modernc.org/sqlite, which
// applies them to every new connection of a pool, so a connection that database/sql opens
// later has the same settings as the first one.
const (
	busyTimeoutMilliseconds = "5000"
	journalModeWAL          = "WAL"
	synchronousNormal       = "NORMAL"
	pragmaEnabled           = "1"
	transactionLockMode     = "immediate"
	// tempStoreMemory keeps the temporary tables of the reader pool in memory: the library
	// listing materialises the files of every top-level item (task M5-11), and in temporary
	// files every page of that work is a system call, which made a 50,000-item facet count
	// several times slower.
	tempStoreMemory = "temp_store(memory)"
	// readerCacheSize raises the page cache of a reader connection from SQLite's 2 MiB to at
	// most 16 MiB (negative: KiB). The cache grows only as pages are read, so an idle
	// connection costs nothing; a large library listing re-reads the same item, file and
	// stream pages, and with the default cache every re-read is a system call (task M5-11).
	readerCacheSize = "cache_size(-16384)"
)

// DSN parameter names understood by modernc.org/sqlite. The driver validates their values
// and applies them in a fixed order (busy_timeout first, query_only last), independent of
// the order in the DSN.
const (
	parameterBusyTimeout = "_busy_timeout"
	parameterForeignKeys = "_foreign_keys"
	parameterJournalMode = "_journal_mode"
	parameterSynchronous = "_synchronous"
	parameterQueryOnly   = "_query_only"
	parameterTxLock      = "_txlock"
	parameterPragma      = "_pragma"
)

const fileURIScheme = "file"

// poolRole selects the role-specific DSN parameters of a pool.
type poolRole int

const (
	// writerRole starts every transaction with BEGIN IMMEDIATE, so a write transaction takes
	// the write lock up front instead of failing with SQLITE_BUSY when it upgrades later.
	writerRole poolRole = iota
	// readerRole connections are query_only: a write sent to the reader pool by mistake fails
	// instead of competing with the single writer.
	readerRole
)

// dataSourceName builds a file: URI for databasePath. A URI keeps Windows drive letters,
// spaces and characters such as '?' or '#' in the path unambiguous: the path is made
// absolute, uses forward slashes and is percent-encoded, e.g.
// file:///C:/Users/Jane%20Doe/server.db?_busy_timeout=5000&... .
func dataSourceName(databasePath string, role poolRole) (string, error) {
	absolutePath, err := filepath.Abs(databasePath)
	if err != nil {
		return "", fmt.Errorf("resolve database path %q: %w", databasePath, err)
	}

	uriPath := filepath.ToSlash(absolutePath)
	if !strings.HasPrefix(uriPath, "/") {
		// A Windows drive path (C:/...) becomes /C:/..., which SQLite maps back to C:\... .
		uriPath = "/" + uriPath
	}

	location := url.URL{Scheme: fileURIScheme, Path: uriPath, RawQuery: connectionParameters(role).Encode()}

	return location.String(), nil
}

func connectionParameters(role poolRole) url.Values {
	parameters := url.Values{}
	parameters.Set(parameterBusyTimeout, busyTimeoutMilliseconds)
	parameters.Set(parameterForeignKeys, pragmaEnabled)
	parameters.Set(parameterJournalMode, journalModeWAL)
	parameters.Set(parameterSynchronous, synchronousNormal)

	switch role {
	case writerRole:
		parameters.Set(parameterTxLock, transactionLockMode)
	case readerRole:
		parameters.Set(parameterQueryOnly, pragmaEnabled)
		parameters.Set(parameterPragma, tempStoreMemory)
		parameters.Add(parameterPragma, readerCacheSize)
	}

	return parameters
}
