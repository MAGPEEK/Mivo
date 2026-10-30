// Package library holds the pure rules for media libraries (ADR 0022): library definitions
// and kinds, the catalog entities, which files count as videos, which folders a scan skips,
// how a title is derived from a file name, safe relative paths, change detection and paging.
// It performs no I/O; adapters walk the file system and persist the results (ADR 0003).
package library
