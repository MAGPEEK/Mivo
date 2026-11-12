// Package transcoder owns the server's view of hardware video encoding: it runs the hardware
// detection in the background at startup and on demand, remembers which encoders failed in a
// session, and hands the playback service the merged server capabilities (ADR 0030 section 5).
package transcoder
