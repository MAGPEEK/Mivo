// Package watcher reports changes below a library root, so a scan can follow new, changed
// and removed files without a manual "Scan" (ADR 0027 §9).
//
// Linux and other systems use fsnotify, which watches single directories (its recursive
// mode is not part of the public API of the pinned v1.10): the watcher walks the root once,
// adds one watch per directory and adds watches for directories created later. The walk
// follows the confinement rules of the library walker: only real directories inside the
// root are watched and links are not followed.
//
// Windows uses one recursive ReadDirectoryChangesW notification on the root instead of
// fsnotify. fsnotify would open a handle per directory, and Windows refuses to rename or
// delete a folder while a handle on anything below it is open, so the owner could no longer
// rename folders in Explorer while the server runs.
//
// On every platform hidden entries and NAS or OS system folders (ADR 0022) are not
// reported, and changes inside them are ignored. Neither backend reports every way a root
// goes away (an unmount on Linux, a rename of the root itself on Windows), so each watch
// also checks every 30 s that the root path still leads to the directory it started with.
package watcher
