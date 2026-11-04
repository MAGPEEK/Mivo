// Package scantrigger keeps libraries current without a manual "Scan" (ADR 0027 §9): it
// starts a scan of a library shortly after its file system watcher reported changes
// (debounced), and again when the last scan of a library is older than the configured
// interval, which also covers network shares that send no events. The scans themselves are
// normal full scans of app/scan. It declares the ports it needs; adapters implement them and
// cmd/mivo wires them (ADR 0003).
package scantrigger
