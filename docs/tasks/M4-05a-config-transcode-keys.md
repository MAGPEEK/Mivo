# M4-05a Config: safe transcode defaults and new keys
Owner: mivo-backend
Depends on: none
Files/areas: `internal/platform/config/**` (+ tests), `docs/` configuration reference

## Goal
Transcoding is gentle by default on a 2-core NAS and every knob of ADR 0030 exists as a key with
validation (AD-25 expert settings).

## Acceptance criteria
- [ ] New keys with env/file/flag parity as for `transcode.max_sessions`: `transcode.preset` (default `veryfast`; allowed
      `ultrafast|superfast|veryfast|faster|fast|medium`), `transcode.threads` (0 = auto, max 64), `transcode.priority`
      (`low|normal`, default `low`), `transcode.throttle_ahead_seconds` (default 60, range 20..600, 0 = off),
      `transcode.hwaccel` (`auto|off|vaapi|qsv|nvenc|amf`, default `auto`), `transcode.hwaccel_device` (path, default empty),
      `transcode.allow_burn_in` (bool, default true).
- [ ] Default of `transcode.max_sessions` is 1 when `runtime.NumCPU() <= 4`, else the existing default; an explicit value wins.
- [ ] Invalid values fail the start with a message naming the key and the allowed values.
- [ ] Tests: table-driven per key (default, valid, invalid, env override), CPU-count default through an injected function.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
- Keep `hwaccel_device` free of path checks beyond "absolute or empty"; the test run (M4-09a) validates it.
- Document keys in the config reference and the Docker README (separate file edits only under `docs/`).
