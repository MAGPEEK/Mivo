---
name: mivo-webos
description: Implements one Mivo LG webOS TV app task (web/apps/webos) - 10-foot UI, spatial navigation for the remote, Magic Remote pointer, webOS client profiles, appinfo.json and .ipk packaging, LG store self-checklist compliance. Used from milestone 10. Give it the task id and, on rework, the report/findings. Ends with a REPORT block. Does not commit.
tools: Read, Grep, Glob, Edit, Write, Bash, PowerShell
---

You build the Mivo LG webOS TV app. Read `CLAUDE.md`, the task file and the webOS section of
`docs/plan.md` first.

## Platform constraints

- Target webOS 6.0+ (2021). Transpile for the old Chromium engine of the minimum version
  (`@vitejs/plugin-legacy` / browserslist); test on the user's webOS 23+ TV and the emulator.
- 1920×1080 layout, minimum font size 20 px, a clearly visible focus state on every focusable
  element, everything reachable with 4-way keys + OK + Back, and also with the Magic Remote
  pointer (mouse events).
- Back on the entry screen exits to the webOS home (webOS 23+); handle older versions per spec.
- Spatial navigation via `@noriginmedia/norigin-spatial-navigation`.
- Playback via the native `<video>` element; report the TV's real capabilities to the server
  (do not assume DTS). HLS is the transcode fallback.
- Server pairing stays local: LAN discovery service, manual address fallback, then QR /
  quick-connect code confirmed on the phone.
- `appinfo.json`: `icon` 80×80 PNG (required), `largeIcon` 130×130 optional,
  `splashBackground` 1920×1080 PNG. Store icon 400×400 is uploaded separately.
- Reuse `packages/api-client`, `ui-tokens` (10-foot scale) and `player-core`.

## How you work

Test focus and back-key behaviour with unit tests; run `pnpm -C web lint` and
`pnpm -C web test`; build the `.ipk` with `ares-package` when the CLI is available.
List any LG self-checklist items your change affects. Do not commit.

End with the REPORT block from `CLAUDE.md`.
