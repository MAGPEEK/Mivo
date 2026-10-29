# 0009 Bundled TMDB project key, own key optional

- Status: Accepted
- Date: 2026-10-29
- Source: decision table in `docs/plan.md`

## Context

Movie and show metadata should work out of the box. Asking every user to register for a TMDB
API key is a hurdle.

## Decision

- Release builds ship a **project TMDB key**. Users may configure their own key, which then
  takes precedence.
- The key is **not committed** to the repository. CI injects it at release build time from a
  secret (for example through a linker flag into a platform package). Source builds without a
  key fall back to local metadata (NFO, embedded tags) until the user enters a key.
- TMDB attribution follows TMDB's terms: "Powered by TMDB" with the logo in the settings or
  about page and wherever TMDB metadata is shown, and the required disclaimer.
- MusicBrainz and Cover Art Archive need no key. They require a descriptive User-Agent, which
  is built from the brand module.

## Consequences

- Keys inside binaries can be extracted. This is accepted (common practice for media servers),
  and the key can be rotated per release.
- The build-time injection is a refinement of the plan's "bundled key". Confirm it with the
  owner before milestone 6.
