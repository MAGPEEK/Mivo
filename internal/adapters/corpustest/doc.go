// Package corpustest holds the milestone 2 promise against the generated test corpus
// (ADR 0021, ADR 0025): every corpus video gets the expected playback mode for the reference
// client profiles of playbacktest and produces playable output.
//
// expectations.go is the one table of expected modes, stream actions, reasons and warnings
// per manifest case name and reference profile. It is ordinary package code, not test code,
// so the playback smoke test in scripts/ checks a running server against the same table. The
// table must agree with the format matrix of internal/domain/playback (M2-09); a
// disagreement is a bug in one of them.
//
// The integration test of this package builds the server's graph itself, like cmd/mivo
// does: a temporary SQLite catalog, the corpus as the only library, the scan with the real
// walker and ffprobe, and playback sessions with the real FFmpeg producer and transcode
// workspace. It plays every video case, joins and probes the HLS output, starts the seek
// clips in the middle, and repeats the decisions for a server without encoders and a browser
// without HLS codecs. It skips with the reason when FFmpeg or the corpus manifest is missing,
// and under -short.
package corpustest
