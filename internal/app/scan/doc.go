// Package scan contains the library scan use case (ADR 0022): walk a library root, probe new
// and changed video files with bounded concurrency, store the results, remove vanished files
// and report progress. Scans run in the background, one per library, and stop when the
// service is closed. It declares the ports it needs; adapters implement them and cmd/mivo
// wires them (ADR 0003).
package scan
