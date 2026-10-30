//go:build windows

package libraryfs

import (
	"encoding/binary"
	"fmt"
	"io/fs"
	"math"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	// Windows has no FIFOs that an open could block on, and os.Root opens directories with
	// plain read access.
	openFileFlags      = os.O_RDONLY
	openDirectoryFlags = os.O_RDONLY

	// fileID128Bytes is the size of FILE_ID_128.
	fileID128Bytes = 16
	// fileIDHalfBytes splits FILE_ID_128 into two little-endian 64-bit halves.
	fileIDHalfBytes = 8
	// fileIndexHighShift places the high half of the 64-bit file index above the low half.
	fileIndexHighShift = 32
)

// fileIDInfo is FILE_ID_INFO, which golang.org/x/sys/windows does not declare.
type fileIDInfo struct {
	VolumeSerialNumber uint64
	FileID             [fileID128Bytes]byte
}

// transientErrors are errors of a drive or network share that may go away when it is back;
// they map to domain.ErrUnavailable.
func transientErrors() []error {
	return []error{
		windows.ERROR_NOT_READY, windows.ERROR_BAD_NETPATH, windows.ERROR_UNEXP_NET_ERR,
		windows.ERROR_NETNAME_DELETED, windows.ERROR_BAD_NET_NAME, windows.ERROR_SEM_TIMEOUT,
		windows.ERROR_DEVICE_NOT_CONNECTED, windows.ERROR_NETWORK_UNREACHABLE,
	}
}

// directoryIdentity returns volume serial number and file id of an open directory. The
// 128-bit id of FileIdInfo is used because ReFS (also Dev Drives) does not guarantee that the
// 64-bit file index is unique; the 64-bit index is the fallback where FileIdInfo is not
// supported (older redirectors, FAT).
func directoryIdentity(directory *os.File, _ fs.FileInfo) (directoryID, error) {
	handle := windows.Handle(directory.Fd())

	var info fileIDInfo

	// The buffer is a Go struct with the exact FILE_ID_INFO layout; its size is passed along.
	err := windows.GetFileInformationByHandleEx(handle, windows.FileIdInfo,
		(*byte)(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))) //nolint:gosec // G103: FILE_ID_INFO buffer for a Win32 call
	if err == nil {
		return identityFromFileIDInfo(info), nil
	}

	return legacyIdentity(handle)
}

// identityFromFileIDInfo converts FILE_ID_INFO; a zero or all-ones (invalid) id is unknown.
func identityFromFileIDInfo(info fileIDInfo) directoryID {
	low := binary.LittleEndian.Uint64(info.FileID[:fileIDHalfBytes])
	high := binary.LittleEndian.Uint64(info.FileID[fileIDHalfBytes:])

	if low == math.MaxUint64 && high == math.MaxUint64 {
		return directoryID{device: info.VolumeSerialNumber}
	}

	return directoryID{device: info.VolumeSerialNumber, indexHigh: high, indexLow: low}
}

func legacyIdentity(handle windows.Handle) (directoryID, error) {
	var information windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &information); err != nil {
		return directoryID{}, fmt.Errorf("read file index: %w", err)
	}

	return identityFromIndex(information.VolumeSerialNumber, information.FileIndexHigh, information.FileIndexLow), nil
}

// identityFromIndex converts a 64-bit file index; FILE_INVALID_FILE_ID (all ones) is unknown.
func identityFromIndex(volume, indexHigh, indexLow uint32) directoryID {
	index := uint64(indexHigh)<<fileIndexHighShift | uint64(indexLow)
	if index == math.MaxUint64 {
		return directoryID{device: uint64(volume)}
	}

	return directoryID{device: uint64(volume), indexLow: index}
}

// videoFileInfo reads the metadata of a regular file found by listing its directory. On
// Windows the listing of the confined directory handle already carries it, without I/O.
func videoFileInfo(_ *os.Root, _ string, entry fs.DirEntry) (fs.FileInfo, error) {
	info, err := entry.Info()
	if err != nil {
		return nil, fmt.Errorf("read entry info: %w", err)
	}

	return info, nil
}
