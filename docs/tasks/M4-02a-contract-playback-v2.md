# M4-02a API contract: playback v2 (tracks, subtitles, quality)
Owner: mivo-backend
Depends on: ADR 0030
Files/areas: `api/openapi.yaml`, generated `internal/adapters/http/apigen`, `web/packages/api-client` (generated only), spec test

## Goal
The contract describes everything the player UI needs for audio/subtitle/quality choice, so
backend and frontend can work in parallel against fakes (ADR 0017, ADR 0030 §3, §4, §6).

## Acceptance criteria
- [ ] `POST /playback/sessions` accepts optional `subtitleTrackId` (string, absent = off),
      `maxBitrate` (int64 bit/s, absent/0 = original) next to the existing `audioStreamIndex`.
- [ ] The session/plan response adds: subtitle decision (`mode` `off|sidecar|burn_in`, `trackId`),
      `outputBitRate`, reason `subtitle_burn_in`, `bitrate_cap`, warning `subtitle_burn_in_unavailable`.
      Unknown-code tolerance stays as in M2 (enums documented as open).
- [ ] Media-file resource lists subtitle tracks (`id`, `language`, `title`, `format`, `forced`,
      `sdh`, `default`, `external`, `textBased`) and audio tracks with `default`.
- [ ] `GET /media-files/{fileId}/subtitles/{trackId}.vtt` (tag `stream`, `text/vtt`, 404 for
      unknown, 422 for image formats) is excluded from the strict server like the other stream routes.
- [ ] Reason codes `dsd_to_pcm`, `hires_downsampled`, `hwaccel_unavailable` are listed; the media-file resource carries
      `defaultAudioStreamIndex` and `defaultSubtitleTrackId` (server policy of M4-04a, nullable).
- [ ] Item detail lists files as versions (`versionLabel`, resolution, bit rate) so DT-07 can pick one.
- [ ] `go generate ./...` and `pnpm -C web gen` leave `git diff --exit-code` clean; the spec test passes.
- [ ] Tests: spec test extended (new fields, enums, route exclusion).
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
- Track ids are stable strings (`s:3` embedded stream index, `x:<hash>` external file), never
  list positions.
- Additive only; v1 clients keep working. Sequential before M4-02b (same file).
