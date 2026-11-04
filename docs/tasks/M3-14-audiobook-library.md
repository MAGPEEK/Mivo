# M3-14 Audiobook library: books with chapters
Owner: mivo-backend (chapter probing: mivo-media)
Depends on: M3-13 (audio classification, tag probing, cover rules)
Files/areas: to be fixed per sub-task; expected: `internal/domain/{library,naming}` (kind
`audiobooks`, item kind `audiobook`), `internal/domain/media` (chapters), `internal/app/scan`,
`internal/adapters/ffmpeg` (chapter probe), `internal/adapters/sqlite` (chapters table
migration), `api/openapi.yaml`, `internal/platform/config`, `scripts/gen-testmedia`

## Goal
An `audiobooks` library lists books with author, narrator and cover; a book opens with its
chapter list, whether it is one M4B file with chapters or a folder of part files. Plan
milestone 3 "Hörbuch-Kapitel", screen 7 data (UI in M5; per-user resume needs M7 or OD-UX-3).
Second wave; ADR 0027 §10.

## Acceptance criteria
- [ ] Config and domain accept `kind: audiobooks`; the contract adds `audiobooks` and item kind
      `audiobook` and a `chapters` list on the item detail (`title`, `startMs`, `endMs`,
      `fileId`).
- [ ] Placement: one book per folder of audio files (parts ordered by disc/track tag, else
      natural file-name order) or per single M4B/M4A/MP3 file in a shared folder; title from
      `album` tag, else folder or file name; author (`artist`/`album_artist`, `composer` as
      narrator when tagged so; NFO/OPF later) as people with roles `author`/`narrator`
      (contract and domain extend `PersonRole`; the schema accepts new roles without a
      rebuild, ADR 0027 §5).
- [ ] Chapters from the file's chapter list (ffprobe `-show_chapters`) or one chapter per part
      file; stored per media file (`media_chapters`, reusable for video chapters in M4).
- [ ] Cover rules as for albums (M3-13).
- [ ] Corpus additions (M4B with chapters – the corpus already has a chapter case, a folder
      book with parts) and integration test; E2E: the book detail lists its chapters.
- [ ] Gates: all Go and web gates of the touched areas.

## Notes
- **Split before implementation** into one-sitting tasks M3-14a … after M3-13 has landed.
- The chapter table is shared with milestone 4 (video chapters, UX PL-30); design it for both
  and record it in ADR 0027 Notes.
- Playback speed (PL-15) and resume per user are M5/M7.
