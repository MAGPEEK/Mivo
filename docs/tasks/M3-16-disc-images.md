# M3-16 Disc images: ISO, BDMV and VIDEO_TS
Owner: mivo-media
Depends on: M3-11b, **OD-M3-5** answered (FFmpeg build and whitelist)
Files/areas: to be fixed per sub-task; expected: `internal/domain/{library,naming,media}`
(layouts), `internal/adapters/fs/libraryfs` (disc folder detection), `internal/adapters/ffmpeg`
(probe and producer input for discs), `internal/app/scan`, `internal/app/playback`,
`internal/adapters/sqlite` (layout values already in the schema), `scripts/gen-testmedia`,
`packaging/docker`, `packaging/synology` (FFmpeg build flags)

## Goal
Unencrypted DVD and Blu-ray backups – ISO files and `VIDEO_TS`/`BDMV` folders – appear in movie
and show libraries as one movie or episode and play through remux or transcode of the main
title. Encrypted discs are listed as unsupported, never decrypted (plan "Disc"). Second wave;
ADR 0027 §10.

## Acceptance criteria
- [ ] The walker reports a directory containing `VIDEO_TS/VIDEO_TS.IFO` or `BDMV/index.bdmv`
      as one disc entry (its files are not listed as separate videos) and `.iso` files as disc
      images; the naming rules place them like a video file named after the folder or ISO.
- [ ] `media_files.layout` is `video_ts`, `bdmv` or `iso`; probing selects the main title
      (longest playlist/title) through the FFmpeg inputs decided in OD-M3-5 (`dvdvideo`
      demuxer, `bluray:` protocol), with the protocol and format whitelists extended only as
      the ADR note allows.
- [ ] Encrypted discs (CSS, AACS) and FFmpeg builds without disc support yield a clear probe
      error code (`disc_encrypted`, `disc_unsupported`), shown as unavailable.
- [ ] Playback always remuxes or transcodes disc sources (no direct play); the decision engine
      gets a reason code for it.
- [ ] Corpus: generated `VIDEO_TS` (FFmpeg `dvd` muxer + generated IFO) or freely licensed
      minimal samples; integration test for detection and probing; playback smoke for one
      disc format where the FFmpeg build supports it.
- [ ] Gates: all Go gates of the touched areas; Docker image and SPK builds with the new FFmpeg
      flags.

## Notes
- **OD-M3-5 decided: stock FFmpeg, concat of pure-Go-selected clips, no libbluray/libdvdnav**
  (ADR 0029; so the criteria above name `concat`, not `dvdvideo`/`bluray:`). **Split
  (2026-11-04):** M3-16a domain, 16b fs detection, 16c title selection, 16d encryption, 16e ISO
  reader, 16f ffmpeg folder input, 16g ffmpeg ISO input, 16h scan + playback, 16i corpus,
  16j packaging verification, 16k wiring.
- The schema already has `media_files.layout` (M3-05), so no core table rebuild is needed.
- Menus, chapters of discs and multiple titles per disc are out of scope.
