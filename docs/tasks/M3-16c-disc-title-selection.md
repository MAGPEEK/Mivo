# M3-16c Disc adapter: main-title selection for BDMV and VIDEO_TS (pure Go)
Owner: mivo-media
Depends on: M3-16a
Files/areas: `internal/adapters/disc` (new package: `bdmv.go`, `videots.go`)

## Goal
Pick the main title of a disc folder without libbluray/libdvdnav.

## Acceptance criteria
- [ ] BDMV: parse `PLAYLIST/*.mpls` (play items, clip names, in/out times) and select the longest playlist that is not a loop (repeated clips) ; resolve `STREAM/*.m2ts` clips in order -> `media.DiscTitle`.
- [ ] VIDEO_TS: group `VTS_nn_k.VOB` (k >= 1) per title set; select the set with the largest total size (duration from `VTS_nn_0.IFO` PGC playback time when parseable); clips in order.
- [ ] Missing clips or unparsable playlists -> `disc_no_title`; all paths validated inside the disc folder (no traversal via crafted playlists).
- [ ] Tests: synthetic MPLS/IFO byte fixtures, fuzz test for the MPLS and IFO parsers, table cases (several playlists, obfuscated short playlists, missing clip).
- [ ] Gates: lint + layer rules + coverage thresholds pass; no new go.mod requirement.

## Notes
Port `DiscTitleResolver` is declared by the consumer in M3-16h. Seamless-branching playlists with multiple angles use angle 1.
