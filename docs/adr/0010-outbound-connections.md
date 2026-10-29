# 0010 No telemetry; the update check is the only automatic outbound connection

- Status: Accepted
- Date: 2026-10-29
- Source: decision table in `docs/plan.md`

## Context

Users run Mivo for privacy and control. They must be able to tell exactly when the server
talks to the internet.

## Decision

- The only connection the server makes **on its own** is a **daily update check** against the
  GitHub Releases API of the project repository. It can be disabled in the settings and the
  config file.
- No telemetry, analytics, crash reporting or usage statistics. Nothing is enabled by default
  and nothing is collected.
- Features the admin configures make outbound connections only when enabled, and these are
  documented: metadata providers (TMDB, MusicBrainz, …), remote access (Let's Encrypt,
  DuckDNS, Tailscale), subtitle search.
- The web UI loads **no third-party assets**. Fonts, icons and scripts are bundled. Exception:
  the Google Cast sender SDK, which must come from Google. It loads only after the user
  enables or uses casting.

## Consequences

- Fonts such as Inter are self-hosted (no Google Fonts CDN).
- Every new outbound feature must say in its task which hosts it contacts and how to turn it
  off.
- The plan's wording ("only a daily update check") is read here as "only *unsolicited*
  connection". Metadata lookups are user-configured features.
