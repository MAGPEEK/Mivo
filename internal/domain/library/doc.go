// Package library holds the pure rules for media libraries (ADR 0022, ADR 0027): library
// definitions and kinds, the catalog entities and their hierarchy (movie, show, season,
// episode, extra), placements with match keys, which files count as videos or sidecars,
// which folders a scan skips, titles and sort titles, safe relative paths, change detection
// and paging. It performs no I/O; adapters walk the file system and persist the results
// (ADR 0003).
//
// Text boundary rule (ADR 0027 §3): file names and NFO text enter this package in Unicode
// NFC. The adapters that read them normalise with golang.org/x/text; this package stays
// standard-library only (ADR 0019) and folds case with unicode and strings.
package library
