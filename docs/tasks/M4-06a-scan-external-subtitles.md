# M4-06a Scan and library FS: find external subtitles
Owner: mivo-backend
Depends on: M4-05b
Files/areas: `internal/adapters/libraryfs` (sidecar listing, new file), `internal/app/scan` (new file + one dispatch line)

## Goal
The scan records external subtitle files of each video file, so they show up as tracks (ADR 0030 §3).

## Acceptance criteria
- [ ] The sidecar lister returns files next to a video and in `Subs/`, `Subtitles/` using `ParseSubtitleName`; reads stay
      inside the library root (`os.Root`), symlinks leaving the root are ignored, at most 64 per video, size ≤ 5 MiB each.
- [ ] The scan hands them to a new optional port `SubtitleCatalog.ReplaceExternalSubtitles(ctx, fileID, []ExternalSubtitle)`;
      unchanged sets cause no write; a removed file removes its row on the next scan.
- [ ] Tests: use case with fakes (add, change, remove, cap), FS integration test on `testdata/library/subs`.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
- Do not open subtitle content here; format sniffing happens at extraction (M4-08a).
- Version labels and part numbers (M3-03) must not be confused with language tokens: `Movie - 4K.en.srt` matches the `Movie - 4K` video.
