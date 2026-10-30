package libraryfs

import (
	"fmt"
	"os"
)

// directoryID identifies a physical directory: device and inode on unix, volume serial number
// and 128-bit file id (or the 64-bit file index as fallback) on Windows. The zero index means
// the file system did not report a usable id.
type directoryID struct {
	device    uint64
	indexHigh uint64
	indexLow  uint64
}

// known reports whether the identity can tell directories apart. Some network, FUSE and pooling
// file systems report zero (or, on Windows, the invalid all-ones id, mapped to zero).
func (identity directoryID) known() bool {
	return identity.indexHigh != 0 || identity.indexLow != 0
}

// identifyDirectory checks that an open handle is a directory and returns its identity.
func identifyDirectory(directory *os.File) (directoryID, error) {
	info, err := directory.Stat()
	if err != nil {
		return directoryID{}, fmt.Errorf("stat directory: %w", err)
	}

	if !info.IsDir() {
		return directoryID{}, fmt.Errorf("%s is not a directory", info.Name())
	}

	return directoryIdentity(directory, info)
}
