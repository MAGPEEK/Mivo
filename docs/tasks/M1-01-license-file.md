# M1-01 Add the GPL-3.0 LICENSE file
Owner: mivo-backend
Depends on: none (but **needs user approval** before any download, see below)
Files/areas: `LICENSE`

## Goal
The repository contains the unmodified official GPL-3.0 license text, matching the claim in
`README.md` and ADR 0001.

## Acceptance criteria
- [ ] **User approval** to download `https://www.gnu.org/licenses/gpl-3.0.txt` was obtained
      through the orchestrator before any network access, and the REPORT records it. Without
      approval, stop with `status: BLOCKED`.
- [ ] `LICENSE` exists at the repository root and is the verbatim downloaded text:
      `(Get-Content LICENSE).Count` is `674`. The file contains `GNU GENERAL PUBLIC LICENSE`
      and `Version 3, 29 June 2007`, and its last non-empty line ends with
      `<https://www.gnu.org/licenses/why-not-lgpl.html>.`
- [ ] `git ls-files --eol LICENSE` (after staging) shows `i/lf`.
- [ ] The REPORT contains the `Get-FileHash -Algorithm SHA256 LICENSE` value. The tester
      compares it with an independent download (with the same approval) or with the
      implementer's value.
- [ ] Gates: none (no code).

## Notes
- Do not rewrap, retitle or add a copyright line to the license text.
- Download with `Invoke-WebRequest -Uri https://www.gnu.org/licenses/gpl-3.0.txt -OutFile LICENSE`
  only after approval. `.gitattributes` normalizes to LF.
- ADR 0001 records "GPL-3.0-or-later". The license text is the same for "only" and
  "or-later". The "or later" choice is expressed in `README.md` and the SPDX identifiers.
