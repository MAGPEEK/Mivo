// Package ffmpeg runs the external FFmpeg tools (ADR 0015): ffprobe describes media files as
// media.Description and finds the keyframe a stream-copy seek starts at; later tasks add
// capability detection and the HLS producer on top of the same process runner.
//
// Process rules, enforced by the runner in process.go: every tool runs through
// exec.CommandContext with an argument slice, never a shell; standard input is the null
// device; standard output is read with a size limit; standard error is kept as a bounded
// tail; the process is killed when its context ends. Every input path is absolute and
// passed as file:<path>, so FFmpeg never reads a file name as an option or a protocol
// (ADR 0022).
//
// ffprobe's JSON stays inside this package; only domain types leave it.
package ffmpeg
