# SPK-03 Package metadata: version mapping, families, templates and env files
Owner: mivo-synology
Depends on: SPK-01, SPK-04 (both done)
Files/areas: `packaging/synology/internal/spkmeta/**` (new Go package + tests)

## Goal
The staging step (SPK-14) can derive everything the toolkit and the server's package scripts
need from the server version and the family – DSM version, beta flag, `arch` list, brand values,
template output – so no packaging file hard-codes the product name, port or version.

## Acceptance criteria
- [ ] Version mapping per ADR 0026 §5 (OD-SPK-5 A), table-tested: `0.1.0-alpha.2`/rev 0 →
      `0.1.0-1020`; rev 1 → `0.1.0-1021`; `0.1.0-alpha.3` → `0.1.0-1030`; `0.1.1-alpha.1` →
      `0.1.1-1010`; `0.1.0-beta.1` → `0.1.0-2010`; `0.1.0-rc.3` → `0.1.0-3030`; `0.1.0` →
      `0.1.0-9000`; `0.0.0-dev` → `0.0.0-0000`; beta flag true for every pre-release. Errors
      (wrapping a package error value) for: leading `v`, build metadata (`+…`), unknown
      pre-release identifiers, pre-release number > 99, revision outside 0–9, non-numeric core,
      any number above 2^31−1.
- [ ] Monotonicity test under the assumed DSM rule (feature parts numerically from the left, then
      the build number; ADR 0026 Notes): a sorted list of at least 10 server versions maps to
      strictly increasing DSM versions. The mapping sits behind one function so the alternative
      "globally monotonic build number" (DSM-NOTES Impact 2) can replace it if SPK-12's upgrade
      test fails.
- [ ] Family table, table-tested, unknown family → error:

      | Family | INFO `arch` | Toolkit platform | `GOARCH` / `GOARM` | Docker platform |
      |---|---|---|---|---|
      | `x86_64` | `x86_64` | `v1000` | `amd64` | `linux/amd64` |
      | `armv8` | `armv8` | `rtd1619b` | `arm64` | `linux/arm64` |
      | `armv7` | `armv7 armada38x monaco` | `alpine` | `arm` / `7` | `linux/arm/v7` |

      plus the FFmpeg family key. The first word of the `arch` value is the family (`INFO.sh` checks
      it against `pkg_get_platform_family`).
- [ ] `build.env` rendering with exactly the fields `INFO.sh` requires (`toolkit/README.md`): `ID`,
      `DISPLAY_NAME`, `DESCRIPTION`, `DESCRIPTION_GER`, `MAINTAINER`, `MAINTAINER_URL`, `VERSION`,
      `DSM_VERSION`, `BETA` (`yes`/`no`), `OS_MIN_VER` (`7.2-64570`), `ARCH`, `PORT`; and
      `package.env` (`ID`, `DISPLAY_NAME`, `ENV_PREFIX`, `PORT`); safely quoted `sh` assignments,
      values from `internal/platform/brand` and `config.DefaultPort`.
- [ ] INFO-safe values (ADR 0026 Notes): every value written to INFO is rejected if it contains
      `"`, `\`, `$`, a backtick, a tab or a newline; `DISPLAY_NAME`, `DESCRIPTION*` and
      `MAINTAINER*` also if they contain `*`, `?`, `[` or two consecutive spaces. UTF-8 (German
      umlauts) is allowed. Tests for each rejected character and for an umlaut that passes.
- [ ] Template rendering: replaces `@ID@`, `@DISPLAY_NAME@`, `@ENV_PREFIX@`, `@PORT@`,
      `@VERSION@`, `@DSM_VERSION@`; a leftover or unknown `@UPPER_CASE@` token is an error naming
      the template and line; `{{wizard_value}}`, `${VAR}` and `$VAR` pass through unchanged; a
      template that renders non-ASCII into `SynoBuildConf/` is rejected (the chroot's Python
      cannot read it).
- [ ] No string literal containing the product name outside `_test.go` files
      (`rg -i -g '!*_test.go' mivo packaging/synology/internal/spkmeta` → no hits).
- [ ] Tests: table-driven unit tests, no mocks; coverage of the package ≥ 90 %
      (`go test -race -cover ./packaging/synology/internal/spkmeta`).
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, `go test ./...`,
      coverage gate, `go mod tidy -diff`.

## Notes
- INFO itself is written by the toolkit (`INFO.sh` reads `build.env`); this package does not
  render INFO. `changelog`, `thirdparty`, `startable` and `adminurl` are not produced.
- Pure functions, standard library only; imports allowed: `internal/platform/brand` and
  `internal/platform/config` (for `DefaultPort`). No new module dependency.
- Descriptions are named constants and describe the media server running on the NAS (opened in a
  browser), not a player app; the German one is real German text.
