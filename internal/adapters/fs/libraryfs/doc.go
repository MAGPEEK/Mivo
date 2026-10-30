// Package libraryfs gives the scanner and the playback adapters read-only access to the files
// below a library root, and to nothing outside it (ADR 0022, path safety).
//
// Every access goes through an os.Root opened on the library root, which refuses ".." and
// symlinks that leave the root. Consequences of that choice:
//
//   - Relative symlinks that stay inside the root are followed, for files and directories.
//   - Symlinks with an absolute target are refused even when the target lies inside the root,
//     because os.Root cannot tell where an absolute path points without leaving the root. On
//     Windows every junction has an absolute target, so junctions are never followed. Use
//     relative links, or add the target folder as its own library (OD-8).
//   - Refused links are skipped by Walk and logged at info level; Resolve and Open report them
//     as domain.ErrForbidden.
//   - Walk lists every physical directory once and follows directory links only after the
//     real tree, so a file is reported under its path without links, and loops or
//     cross-linked folders cannot multiply the walk. A file link is an alias the user made and
//     is reported under its own path.
//
// Walk never fails because of one unreadable folder: it logs it and returns its path, so the
// scan keeps the catalog entries below it. Only a root that cannot be read, also when it
// disappears mid-walk (a dropped NAS share), fails the walk with domain.ErrUnavailable.
//
// Files are opened without blocking and checked on the open handle, directories with
// O_DIRECTORY on unix, so a FIFO named like a video can never hang a scan or a request.
//
// Relative paths are the catalog's canonical slash-separated form (library.ParseRelativePath).
// Before a path is joined to the root it must also be local for the host OS
// (filepath.IsLocal: no Windows device names such as NUL or COM1), and on Windows no segment
// may end in a dot or space or contain a colon, so "movie.mkv." or "movie.mkv:stream" cannot
// alias "movie.mkv" or reach an alternate data stream. Walk logs and skips entries whose path
// breaks these rules instead of failing the scan, so every reported path can be resolved.
//
// The package never writes: Docker mounts the media read-only, and the server must not change
// a library (ADR 0022). Paths in log records are relative to the library root.
package libraryfs
