# M2-04 Configuration: libraries, FFmpeg tools and transcode limits
Owner: mivo-backend
Depends on: none (library keys follow OD-1, transcode default follows OD-4)
Files/areas: `internal/platform/config/**`, `internal/platform/paths/**`,
`docs/adr/0018-configuration-and-os-paths.md` (append to "Notes" only)

## Goal
Operators can point the server at their video folders, at a specific FFmpeg build and set the
transcode limit through `config.yaml` or environment variables, with the same strict validation
as the milestone-1 keys (ADR 0018, ADR 0022).

## Acceptance criteria
- [ ] New keys, each with YAML key, env name, default and validation in the code table and in
      the REPORT:
      `libraries` (list of `{name, path, kind}`; `kind` defaults to `movies`),
      `<PREFIX>_LIBRARIES` (OS path list via `filepath.SplitList`, replaces the YAML list, names
      from the last path element), `tools.ffmpeg` / `tools.ffprobe` (default `ffmpeg` /
      `ffprobe`), `transcode.max_sessions` (integer 1–16, default per OD-4; until OD-4 is
      answered use 2).
- [ ] Library paths resolve like other paths (relative → config dir, `~` rejected, drive-relative
      rejected). Errors name the key, list index and file, e.g.
      `config.yaml: libraries[1].path: ...`.
- [ ] Rejected with a clear error: duplicate roots, a root nested in another root, a root equal
      to / containing / inside the config, data, cache or transcode directory; comparison is
      case-insensitive on Windows (test with a Windows-style injected environment on any host).
- [ ] A non-existent library root is accepted (not checked here, ADR 0022).
- [ ] `tools.*`: a bare command name stays as is (PATH lookup happens at startup in M2-32); a
      value with a path separator is resolved as a path. Empty value → error.
- [ ] Unknown keys inside a library entry are reported like other unknown keys
      (`line N: unknown key "libraries[0].pth"`).
- [ ] BACKLOG row "config/paths" is done: drive-relative `C:cfg` config dir rejected
      (`ErrDriveRelative`) for flag and env; ADR 0018 Notes get the `yaml.Node` walk / empty-section
      note; `ErrInvalid` wrapping nits fixed (invalid YAML, multiple documents, duplicate key,
      section in non-scalar-key error, `%w` only for non-nil default error, `otherOS` helper).
- [ ] Tests: table tests for every key (default < YAML < env), list parsing, overlap rules,
      Windows case-insensitivity; coverage of both packages stays ≥ 95 %.
- [ ] Gates: lint + layer rules + `go test -race ./internal/platform/...` + coverage gate pass.

## Notes
- Multi-word keys use snake_case (ADR 0022) so the env name is unambiguous.
- The config struct gains `Libraries []LibraryConfig`, `Tools{FFmpeg, FFprobe}`,
  `Transcode{MaxSessions}`. `platform` must not import `domain`; `cmd` (M2-32) converts to
  `library.Definition`.
- The YAML list is a new shape for the node walk: keep the explicit key table approach, add a
  small list-of-mappings handler rather than reflection.
- Do not wire anything in `cmd` here.

## Orchestrator note (2026-10-30)
- `transcode.max_sessions` is validated as 1–8 instead of 1–16: ADR 0024 caps all FFmpeg
  sessions at 8, so larger values would have no effect.
- The Docker preset `MIVO_LIBRARIES=/media` (OD-1) belongs to M2-35, not to this task.
