// Package stream delivers the bytes of playback sessions (ADR 0024): the original file of a
// direct-play session with Range support, and the playlist, init segment and media segments
// of a remux or transcode session. Its handlers are plain net/http handlers mounted on the
// REST API mux (restapi.Mount), so they keep the API's 404 and 405 problems while using
// Range requests and long-lived writes the generated strict server cannot express.
//
// Path safety (ADR 0022, ADR 0024): no path from the request reaches the file system. The
// file route serves only the library file the session grants, opened through os.Root of its
// library root. The HLS route accepts exactly three name shapes (index.m3u8, init.mp4,
// seg-NNNNN.m4s) and opens them through os.Root of the session's own output directory, so
// one session can never read another session's files or anything else.
//
// The route patterns and URLBuilder are the only place that knows the stream paths. Clients
// load the streamUrl the server returns and follow the playlist's relative URIs, so a token
// path segment can be added in milestone 7 without changing the handlers' logic.
package stream
