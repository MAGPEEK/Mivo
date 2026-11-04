# M4-05b Domain: external subtitle files
Owner: mivo-backend
Depends on: none
Files/areas: `internal/domain/media/subtitle.go`, `internal/domain/library` naming helper (+ tests)

## Goal
A subtitle file next to a video becomes a subtitle track with language and flags, deterministically,
without I/O (ADR 0030 §3).

## Acceptance criteria
- [ ] `ParseSubtitleName(videoBase, fileName) (ExternalSubtitle, bool)` matches `Movie.en.srt`, `Movie.eng.forced.srt`,
      `Movie.pt-BR.sdh.ass`, `Movie.srt` (language `und`), rejects other basenames, handles `Subs/<video base>.es.srt`.
- [ ] Recognised formats: srt, ass, ssa, vtt, sub+idx pair (image, flagged non-text), sup (PGS, image).
- [ ] `ExternalSubtitle{RelativePath, Format, Language, Forced, SDH, Default, TextBased}` and a stable track id `x:<sha1 prefix of relative path>`.
- [ ] Unicode NFC names and case-insensitive extensions.
- [ ] Tests: table-driven, coverage ≥ 90 %.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
- Language list: ISO 639-1/-2 codes plus region tags; unknown tokens stay part of the title, not an error.
