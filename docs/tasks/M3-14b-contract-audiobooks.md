# M3-14b Contract: audiobooks and chapters
Owner: mivo-backend
Depends on: M3-13b
Files/areas: `api/openapi.yaml`, generated Go and TS clients, spec test

## Goal
The API describes audiobook libraries, books and their chapters.

## Acceptance criteria
- [ ] `LibraryKind` gains `audiobooks`; `ItemKind` gains `audiobook`; `PersonRole` gains `author|narrator`.
- [ ] Item detail has optional `chapters[]` (`title`, `startMs`, `endMs`, `fileId`).
- [ ] Generation leaves `git diff --exit-code` clean; spec test asserts the additions.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Additive; no handler logic (M3-14g fills `chapters`).
