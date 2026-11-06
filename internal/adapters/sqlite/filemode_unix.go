//go:build unix

package sqlite

import (
	"io/fs"
	"os"
	"syscall"
)

// fileOwnedByProcess reports whether the effective user of the process owns the file, the
// only case in which chmod can succeed without extra privileges.
func fileOwnedByProcess(info fs.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)

	return ok && int64(stat.Uid) == int64(os.Geteuid())
}
