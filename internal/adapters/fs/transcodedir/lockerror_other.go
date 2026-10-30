//go:build !windows

package transcodedir

// isLockError always reports false: Unix-like systems delete files that are still open, so a
// failed deletion does not succeed by waiting and is returned at once.
func isLockError(error) bool {
	return false
}
