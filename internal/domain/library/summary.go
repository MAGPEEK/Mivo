package library

// Summary is a library with the number of its items, as the library list and the library
// detail show it.
type Summary struct {
	Library   Library
	ItemCount int64
}

// SyncResult is the outcome of syncing the configured definitions into the catalog.
// Libraries lists every library that is configured now, ordered by name. Removed lists the
// libraries that were deleted because their root is no longer configured, each with the
// number of items deleted with it, so the caller can report what a configuration change
// (or a typo in it) removed.
type SyncResult struct {
	Libraries []Library
	Removed   []Summary
}
