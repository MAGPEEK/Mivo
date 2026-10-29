# 0011 Design direction: Plex-like layout, own identity, coral accent

- Status: Accepted
- Date: 2026-10-29
- Source: design section in `docs/plan.md`

## Context

Users coming from Plex expect a familiar layout. Mivo must not use Plex branding and must be
clearly distinguishable from it.

## Decision

- Dark-first theme (light theme optional later). Core tokens:
  background `#0E1014`, surface `#181B21`, raised `#22262E`, divider `#2C313A`,
  text `#E8EAED`, muted text `#9AA0A6`,
  accent **coral `#FF6B57`** (hover `#FF8572`, text on accent `#3A0E07`).
- Status colours: direct play green, direct stream blue, transcode amber. Exact values live
  in `ui-tokens`.
- Font **Inter** (self-hosted, ADR 0010). Radii 6–10 px, subtle shadows, cards lift on hover.
- Layout: collapsible left sidebar (bottom navigation on phones), top bar with global search
  (Ctrl+K), home with hero area and dense horizontal rows.
- All colours, spacing and type come from `web/packages/ui-tokens`. Components contain no raw
  hex values, and lint enforces this. The tokens include a web scale and a 10-foot (TV) scale.
- Text colour pairs meet WCAG AA contrast (≥ 4.5:1). A unit test enforces this.

## Consequences

- No Plex logos, names or orange (`#E5A00D`).
- Changing the theme means changing tokens, not components.
