# SPK-03 Package metadata: version mapping, families, templates and env files
Owner: mivo-synology
Depends on: SPK-01 (version comparison, INFO quoting), SPK-04 (platform per family, `build.env`
fields)
Files/areas: `packaging/synology/internal/spkmeta/**` (new Go package + tests)

## Goal
The staging step (SPK-14) can derive everything the toolkit and the server's package scripts
need from the server version and the family – DSM version, beta flag, brand values, template
output – so no packaging file hard-codes the product name, port or version.

## Acceptance criteria
- [ ] Version mapping per ADR 0026 §5 (OD-SPK-5 A), table-tested: `0.1.0-alpha.2`/rev 0 →
      `0.1.0-1020`; rev 1 → `0.1.0-1021`; `0.1.0-beta.1` → `0.1.0-2010`; `0.1.0-rc.3` →
      `0.1.0-3030`; `0.1.0` → `0.1.0-9000`; `0.0.0-dev` → `0.0.0-0000`; beta flag true for every
      pre-release. Errors (wrapping a package error value) for: leading `v`, build metadata
      (`+…`), unknown pre-release identifiers, pre-release number > 99, revision outside 0–9,
      non-numeric core.
- [ ] Monotonicity test: a sorted list of at least 10 server versions (alphas, betas, rcs, finals,
      patch and minor bumps) maps to strictly increasing DSM versions under the comparison rule
      SPK-01 documents (implemented in the test as a comparator).
- [ ] Family table: exactly `x86_64`, `armv8`, `armv7`, each with INFO `arch` value, toolkit
      platform (from SPK-04), `GOARCH`, `GOARM` (armv7 only), Docker platform and the
      `sources.lock` family key; unknown family → error. Table-tested.
- [ ] `build.env` rendering (fields per SPK-04: at least `ID`, `DISPLAY_NAME`, `DESCRIPTION`,
      `DESCRIPTION_GER`, `MAINTAINER`, `MAINTAINER_URL`, `VERSION`, `DSM_VERSION`, `BETA`,
      `OS_MIN_VER`, `ARCH`, `PORT`) and `package.env` rendering (`ID`, `DISPLAY_NAME`,
      `ENV_PREFIX`, `PORT`) as safely quoted `sh` assignments; values from
      `internal/platform/brand` and `config.DefaultPort`. Tests: a display name with a space and an
      apostrophe round-trips (Go-side quoting test, plus one `sh -c '. ./file; printf %s …'` check
      in the REPORT); values with a newline are rejected; values the INFO format cannot carry
      (SPK-01) are rejected.
- [ ] Template rendering: replaces `@ID@`, `@DISPLAY_NAME@`, `@ENV_PREFIX@`, `@PORT@`,
      `@VERSION@`, `@DSM_VERSION@`; a leftover or unknown `@UPPER_CASE@` token is an error naming
      the template and line; `{{wizard_value}}`, `${VAR}` and `$VAR` pass through unchanged
      (tests for each).
- [ ] No string literal containing the product name outside `_test.go` files
      (`rg -i -g '!*_test.go' mivo packaging/synology/internal/spkmeta` → no hits).
- [ ] Tests: table-driven unit tests, no mocks; coverage of the package ≥ 90 %
      (`go test -race -cover ./packaging/synology/internal/spkmeta`).
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, `go test ./...`,
      coverage gate, `go mod tidy -diff`.

## Notes
- INFO itself is written by the toolkit (`INFO.sh` from SPK-04 reads `build.env`); this package
  does not render INFO.
- Pure functions, standard library only; imports allowed: `internal/platform/brand` and
  `internal/platform/config` (for `DefaultPort`). No new module dependency.
- `os_min_ver`, description texts (describe the media server, not a player app) and the
  maintainer are named constants with a reference to ADR 0026.
- If SPK-01 shows DSM compares versions differently, implement the rule SPK-01 found and report
  the deviation; do not invent a new scheme.
