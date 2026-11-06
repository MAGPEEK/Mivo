//go:build !unix

package sqlite

import "io/fs"

// fileOwnedByProcess reports false: without Unix ownership the mode is never changed (on
// Windows the files inherit the data directory ACL, see databaseFiles.prepare).
func fileOwnedByProcess(fs.FileInfo) bool {
	return false
}
