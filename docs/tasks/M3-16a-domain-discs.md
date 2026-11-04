# M3-16a Domain: disc layouts, error codes, naming
Owner: mivo-backend
Depends on: M3-04, M3-03
Files/areas: `internal/domain/media` (layout values, `DiscClip`, error codes), `internal/domain/naming` (new file `disc.go`), `internal/domain/playback` or the decision package (reason code only)

## Goal
Pure types and rules for discs so later tasks share one vocabulary.

## Acceptance criteria
- [ ] `media.Layout` values `file|video_ts|bdmv|iso` (matching `media_files.layout`); `media.DiscClip` (path or ISO-relative extent, duration); `DiscTitle` = ordered clips.
- [ ] Error codes `disc_encrypted`, `disc_unsupported`, `disc_no_title` as domain errors.
- [ ] Decision reason code `disc_source` (never direct play) added to the decision engine's reason enum.
- [ ] Naming: a disc named by its folder (`Movie (2019)/BDMV`, `Movie (2019)/VIDEO_TS`) or ISO file name is parsed like a video file of that name, including show episode patterns.
- [ ] Tests: table-driven, domain coverage >= 90 %.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Menus, disc chapters and multiple titles stay out of scope. Decisions: ADR 0029.
