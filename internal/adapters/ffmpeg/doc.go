// Package ffmpeg runs the external FFmpeg tools (ADR 0015): ffprobe describes media files as
// media.Description and finds the keyframe a stream-copy seek starts at; ffmpeg reports its
// capabilities and, as the Producer, writes the HLS output of a playback session (ADR 0024),
// all on top of the same process runner.
//
// On Linux the Producer hands FFmpeg the media file as an inherited descriptor
// (/proc/self/fd/3) opened through os.Root of the library, so a path swapped after the
// library check cannot redirect FFmpeg (ADR 0022). Other systems pass the resolved path.
//
// Process rules, enforced by the runner in process.go: every tool runs through
// exec.CommandContext with an argument slice, never a shell; standard input is the null
// device; standard output is read with a size limit; standard error is kept as a bounded
// tail; the process is killed when its context ends, and with the server where the system
// allows it (Linux parent-death signal, Windows job object; lifetime_*.go). Every input path
// is absolute and passed as file:<path>, so FFmpeg never reads a file name as an option or a
// protocol, behind -protocol_whitelist file and a -format_whitelist of demuxers that read
// only the one file, so a hostile file cannot make FFmpeg open the network or other files
// (ADR 0022).
//
// ffprobe's JSON stays inside this package; only domain types leave it.
package ffmpeg
