//go:build windows

package folderbrowse

import "golang.org/x/sys/windows"

// transientErrors are errors of a drive or network share that may go away when it is back
// (same set as adapters/fs/libraryfs).
func transientErrors() []error {
	return []error{
		windows.ERROR_NOT_READY, windows.ERROR_BAD_NETPATH, windows.ERROR_UNEXP_NET_ERR,
		windows.ERROR_NETNAME_DELETED, windows.ERROR_BAD_NET_NAME, windows.ERROR_SEM_TIMEOUT,
		windows.ERROR_DEVICE_NOT_CONNECTED, windows.ERROR_NETWORK_UNREACHABLE,
	}
}
