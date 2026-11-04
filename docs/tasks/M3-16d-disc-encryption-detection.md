# M3-16d Disc adapter: encryption detection (never decrypt)
Owner: mivo-media
Depends on: M3-16c
Files/areas: `internal/adapters/disc` (new file `encryption.go`)

## Goal
Recognise CSS- and AACS-protected discs and mark them unavailable.

## Acceptance criteria
- [ ] DVD: sector/PES scrambling flags in the first main-title VOB sectors (pack header scrambling control) -> `disc_encrypted`.
- [ ] Blu-ray: presence of `AACS/` with a main-title m2ts whose TP_extra_header copy-permission bits or transport scrambling flags are set, or a `BDMV/STREAM` file whose first packets are not valid sync bytes -> `disc_encrypted`.
- [ ] No key handling, no libdvdcss, no decryption path anywhere in the repo.
- [ ] Tests: synthetic sector fixtures for scrambled/clear DVD and m2ts; unencrypted AACS-less disc passes.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Heuristic, documented as such: a false "encrypted" is shown as unavailable, never as broken playback. ISO sources are checked after the ISO reader (M3-16e) by the same functions over an `io.ReaderAt`.
