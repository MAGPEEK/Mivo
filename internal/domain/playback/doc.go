// Package playback decides how a media file reaches a client (ADR 0023): Direct Play (the
// original file), Remux (every stream copied into fMP4 HLS) or Transcode (at least one stream
// re-encoded), together with the action per stream and the reasons and warnings the
// stream-info overlay shows.
//
// Decide is a pure function of the technical description of the file (domain/media), the
// client profile, the server capabilities, the request and the policy. It never runs FFmpeg
// and knows no FFmpeg arguments; adapters/ffmpeg translates a Plan into a command line.
//
// The rules are small named checks (direct-play check, copy check, transcode targets); each
// check contributes its own reasons, so the reason list explains every cheaper mode that was
// ruled out without a central switch.
package playback
