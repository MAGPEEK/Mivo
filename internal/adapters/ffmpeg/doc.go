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
// protocol.
//
// Every input also passes two restrictions, so a hostile file can make FFmpeg open neither
// the network nor other files (ADR 0022): -protocol_whitelist file keeps every URL FFmpeg
// opens for it, nested opens included, on the local file system, and a -format_whitelist of
// demuxers that read only the one file keeps playlist-like demuxers (concat, hls, dash, imf)
// from opening the files the content names. Windows device paths that do not name a file on a
// drive or a UNC share are refused before FFmpeg runs (ADR 0022 Notes M2-39).
//
// ffprobe's JSON stays inside this package; only domain types leave it. FFmpeg's error
// output stays inside as well: errors name the tool and its exit code only, and the stderr
// tail goes to the server log (debug for a failed probe or keyframe lookup, warn for a
// failed capability detection or stream production; ADR 0024 Notes M2-39).
package ffmpeg
