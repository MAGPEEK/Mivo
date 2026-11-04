// Package folderbrowse lists the readable sub-directories of configured browse roots and
// checks a chosen folder (ADR 0028). Every access goes through os.Root, so a link or a
// ".." cannot leave the root, and no result ever contains an absolute path. The helpers for
// path rules and availability follow adapters/fs/libraryfs (path.go, availability.go); they
// are duplicated on purpose because moving them would touch the scan lane.
package folderbrowse
