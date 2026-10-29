# 0001 License: GPL-3.0-or-later

- Status: Accepted
- Date: 2026-10-29
- Source: decision table in `docs/plan.md`

## Context

Mivo is open source for everyone. It drives a GPL-licensed FFmpeg build (ADR 0015) and
should stay open when forked or redistributed, including in NAS packages and Docker images.

## Decision

- The project is licensed **GPL-3.0-or-later** (SPDX identifier). `LICENSE` holds the
  unmodified official GPL-3.0 text from gnu.org.
- No per-file license headers. The SPDX identifier appears in `README.md`, in every
  `package.json` (`"license": "GPL-3.0-or-later"`) and in the OCI image label.
- Dependencies must be GPL-3.0-compatible (MIT, BSD, ISC, Apache-2.0, MPL-2.0, LGPL, GPL-3.0).
  New dependencies are license-checked in review.
- No code from GPL-2.0-only projects (for example Jellyfin) is copied. Mivo is an independent
  implementation.
- Distributed artifacts that bundle third-party binaries (FFmpeg) ship their license notices
  and a source reference.

## Consequences

- Forks and redistributors must publish their source under the GPL.
- GPL-2.0-only code cannot be reused, which reinforces the clean-room rule.
- The plan says "GPL-3.0" and the README says "GPL-3.0-or-later". This ADR settles on
  **or-later**. If the owner wants GPL-3.0-only, supersede this ADR.
