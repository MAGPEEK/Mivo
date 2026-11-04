# M3-16h Scan and playback integration for discs
Owner: mivo-backend
Depends on: M3-16b, M3-16d, M3-16f, M3-10b
Files/areas: `internal/app/scan` (new file `disc.go`, one dispatch line), `internal/app/playback` (source selection, reason code), `internal/adapters/sqlite` (layout value only, no migration)

## Goal
Discs become movies or episodes, are probed via their main title and always play through remux or transcode.

## Acceptance criteria
- [ ] Ports `DiscTitleResolver` and `DiscProber` declared in `app/scan`; a disc entry becomes one media file with `layout` and the main-title streams.
- [ ] `disc_encrypted`/`disc_unsupported`/`disc_no_title` are stored on the media file and surfaced as `unavailable` with the code in the item detail (contract field already reserved by M3-04; add the enum values if missing).
- [ ] Playback decision: disc sources never direct-play; reason `disc_source`; transcode needed for bitmap subtitles is reported as "not supported" until M4.
- [ ] Tests: fakes for resolver/prober; scan places `Movie (2019)/BDMV` and `Show S01E02.iso`; encrypted disc is unavailable; decision table rows.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
ISO resolution (M3-16g) is injected optionally; without it ISO entries are `disc_unsupported`, so folder discs can ship first.
