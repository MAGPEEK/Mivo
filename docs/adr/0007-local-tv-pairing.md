# 0007 Local TV pairing (discovery + Quick Connect)

- Status: Accepted
- Date: 2026-10-29
- Source: decision table and webOS section in `docs/plan.md`

## Context

Typing credentials on a TV remote is painful. A cloud pairing service would contradict
ADR 0006 and ADR 0010.

## Decision

- The TV app finds the server in the LAN through a bundled webOS JS service (SSDP or UDP
  discovery). Manual address entry is the fallback.
- Login happens through a QR code or a short **Quick Connect** code on the TV. The user
  confirms it in a browser on their phone, on their own server. The TV then receives its
  token.
- No online pairing service for now. An official online instance may come later as an
  opt-in option, through a new ADR.
- The server sends CORS headers for the webOS app origin.

## Consequences

- Pairing works fully offline inside the home network.
- The TV must reach the server directly (LAN, or remote access path A from the plan).
- The server needs a Quick Connect endpoint and a discovery responder (milestones 7 and 10).
