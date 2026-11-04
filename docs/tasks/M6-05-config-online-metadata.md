# M6-05 Config: online switches, keys, build-time TMDB key
Owner: mivo-backend
Depends on: none
Files/areas: `internal/platform/config/**`, `internal/platform/brand/**` (user-agent only), docs `docs/development.md` (build flag)

## Goal
Configuration for the online features with safe defaults and the bundled-versus-own key rule
of ADR 0031 §2-§3.

## Acceptance criteria
- [ ] Keys with env overrides: `metadata.online.enabled` (default true), `metadata.tmdb.enabled`,
      `metadata.tmdb.api_key`, `metadata.musicbrainz.enabled`, `metadata.language`
      (default `en`), `MIVO_` env equivalents; documented in the config reference.
- [ ] A package variable `BundledTMDBKey` set via `-ldflags "-X …"`; empty in dev builds.
      `config.TMDBKey()` returns `(key, origin)` with origin `none|bundled|own`, own wins.
- [ ] The key never appears in `String()`/log output/`/api` config dumps (test with a
      logging capture).
- [ ] Release docs (`docs/development.md`) explain how CI injects the project key; Dockerfile
      and SPK build keep working without it.
- [ ] Tests: table tests for precedence file < env, own > bundled, redaction.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Do not commit a real key. The release workflow passing the key is a note for `mivo-release`,
not part of this task.
