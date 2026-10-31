# 0002 "Mivo" is a working title with a single configurable source

- Status: Accepted
- Date: 2026-10-29
- Source: decision table in `docs/plan.md`

## Context

"Mivo" collides with the Indonesian app "Mivo TV". The final name is chosen before the first
public release, so renaming must be cheap.

## Decision

- User-visible names and name-derived values come from exactly two modules:
  - Go: `internal/platform/brand` (display name, lowercase id, env prefix, repository URL)
  - Web: `web/packages/ui-tokens/src/brand.ts` (product name, id, repository URL)
- Everything else derives from them: UI strings, page title, log fields, OS directory names,
  environment variable prefix, User-Agent, storage keys.
- Code identifiers never contain the name (`createApiClient`, not `createMivoClient`).
- Some **structural identifiers** cannot read a constant. They may contain the working title
  and form the rename checklist:
  1. Go module path (ADR 0016) and the `cmd/mivo` directory / binary name
  2. npm workspace scope `@mivo/*`
  3. `api/openapi.yaml` `info.title`
  4. Docker image name, the `MIVO_*` variables and the OCI labels in `packaging/docker/`
  5. Docker Hub repository `docker.io/magpeek/mivo-media-server` (published since
     0.1.0-alpha.1 – a rename needs a new repository and a deprecation note on the old one)
  6. Docs that name the product or derived values: `README.md`, `CLAUDE.md`,
     `docs/development.md`, `packaging/docker/README.md`
  7. Later: webOS app id, GoReleaser config, installer names, systemd unit

## Consequences

- A rename means editing two brand files plus the checklist above.
- Reviewers reject new hard-coded occurrences. A quick check is a case-sensitive search for
  `Mivo` in `*.go`, `*.ts` and `*.tsx` files: the only expected hits are the brand modules and
  tests that assert on them.

## Notes (ADR 0026, Synology package)

Additions to the rename checklist for the native Synology package (ADR 0026); they
extend items 4, 6 and 7 without changing the decision.

- **DSM package name**: INFO `package` is `brand.ID`, filled in at build time. The same name is
  the package's DSM system user, the directory `/var/packages/<id>/`, the firewall service file
  `<id>.sc` and the prefix of the SPK file names. DSM identifies a package by this name: a
  renamed package installs as a different package, existing installs get no upgrade, and the
  release notes must explain moving the data in `/var/packages/<old id>/var` and granting the
  shared folders to the new package user again.
- **Docs** that name the product: `packaging/synology/README.md` and
  `packaging/synology/TESTING.md`.
- Templates under `packaging/synology/package/` never contain the name; they use build-time
  tokens (`@ID@`, `@DISPLAY_NAME@`, `@ENV_PREFIX@`) and generated `package.env` / `build.env`
  files that the staging step (`packaging/synology/cmd/spkstage`) fills from
  `internal/platform/brand`. The toolkit project directory name is the brand id as well. A case-insensitive search for the name in that directory must
  find nothing.
