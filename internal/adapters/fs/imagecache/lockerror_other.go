//go:build !windows

package imagecache

// isLockError always reports false: Unix-like systems rename over and delete open files, so
// waiting never turns a failure into success.
func isLockError(error) bool {
	return false
}
