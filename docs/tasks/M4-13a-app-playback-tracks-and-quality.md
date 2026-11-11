# M4-13a App playback: tracks, subtitles, quality and encoder in sessions
Owner: mivo-backend
Depends on: M4-04a, M4-04b, M4-04c, M4-04d, M4-02a
Files/areas: `internal/app/playback/start.go`, `ports.go`, `session.go` (+ tests, fakes)

## Goal
Creating a session honours audio track, subtitle choice, quality cap and the chosen encoder, and reports them (ADR 0030 §3 – §6; closes the
BACKLOG row "sourceBitRate always omitted").

## Acceptance criteria
- [ ] `StartRequest` carries `SubtitleTrackID`, `MaxBitrate`, `AllowDegradeSubtitles`; the service builds the decision `Input` with the file's embedded and external tracks
      (new optional port `SubtitleTracks.Tracks(ctx, fileID)`), applies the default policy of M4-04a when absent.
- [ ] `Job` carries burn-in track reference, bit-rate cap, encoder; `Session` exposes `SubtitleMode`, `SubtitleTrackID`, `OutputBitRate`,
      `SourceBitRate`.
- [ ] Burn-in sessions count as re-encoding sessions; at the limit, the subtitle drops to off with warning `subtitle_burn_in_unavailable`
      instead of a 503 when the request allows it (`allowDegradeSubtitles`); otherwise 503 as before.
- [ ] `replacesSessionId` still swaps atomically when track, subtitle or quality changes. Burn-in feasibility (limit, transcoding enabled)
      is decided BEFORE the replaced session is stopped, counting it as freed; a start that then fails (503/422) leaves the replaced
      session running. Test: replace + burn-in refused without `allowDegradeSubtitles` → old session still serves.
- [ ] Hardware start failure (producer `Fallback=true`) marks the encoder degraded through a `HardwareStatus` port.
- [ ] Tests: use cases with fakes (every combination of table: sub off/sidecar/burn-in × limit free/reached × replace); race tests.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
- Subtitle sidecar choice does not touch the producer; only the plan reports it.
- Keep `start.go` under the function-size rule: extract an `inputBuilder`.
