# M1-02 Go module, layered skeleton, brand and build info
Owner: mivo-backend
Depends on: none
Files/areas: `go.mod`, `go.sum`, `cmd/mivo/main.go`, `internal/domain/doc.go`,
`internal/app/doc.go`, `internal/adapters/doc.go`, `internal/platform/doc.go`,
`internal/platform/brand/**`, `internal/platform/buildinfo/**`

## Goal
The Go module exists with the layered directory structure. `mivo --version` prints the
product name and version. Product name and build metadata each have exactly one source.

## Acceptance criteria
- [ ] `go.mod` declares `module github.com/magpeek/mivo` (ADR 0016). The `go` directive is
      the installed stable Go version and at least `1.25`.
- [ ] `go.mod` contains an `ignore` directive for `./web`. `go list ./...` lists no package
      under `web/`.
- [ ] `internal/{domain,app,adapters,platform}/doc.go` each have a package doc comment that
      states the layer's import rule (ADR 0003).
- [ ] `internal/platform/brand` exports constants for display name (`Mivo`), lowercase id
      (`mivo`), env prefix (`MIVO`) and repository URL (`https://github.com/MAGPEEK/Mivo`).
      `Select-String -Path (Get-ChildItem -Recurse -Filter *.go) -Pattern '"Mivo"' -CaseSensitive`
      finds the literal only in `internal/platform/brand` (and its test).
- [ ] `internal/platform/buildinfo` exposes `Read() Info` with `Version`, `Commit`,
      `CommitTime`, `Modified` and `GoVersion`. Version resolution order: linker override >
      main module version from `runtime/debug.ReadBuildInfo` (ignoring `(devel)`) >
      `0.0.0-dev`. Commit data comes from the `vcs.*` build settings.
- [ ] `go run ./cmd/mivo --version` prints one line starting with `Mivo ` and exits 0.
- [ ] `go build -ldflags "-X github.com/magpeek/mivo/internal/platform/buildinfo.version=9.9.9" -o bin/mivo.exe ./cmd/mivo; ./bin/mivo.exe --version`
      prints a line containing `9.9.9`.
- [ ] `go run ./cmd/mivo --unknown-flag` exits non-zero with a usage message on stderr and
      no panic.
- [ ] Tests: table-driven tests for version resolution. The resolution logic is a pure
      function taking `*debug.BuildInfo` and the override string. Brand test: id is
      lowercase, env prefix is the upper-case id.
- [ ] Gates: `go vet ./...` and `go test ./...` pass. golangci-lint is configured in M1-03,
      which will also fix findings in these files.

## Notes
- `main.go` stays tiny: `func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }`
  style. Parse flags with the standard `flag` package. M1-10 extends it later. Keep the
  `run` signature easy to extend (later it takes a context and an env lookup).
- The linker-overridable `version` variable is the one accepted package-level variable. Mark
  it with a `//nolint:gochecknoglobals // set by the linker` comment. Expose it only through
  `Read()`.
- `go run` does not stamp VCS info, so `0.0.0-dev` is expected there. `go build` from a
  checkout stamps a pseudo-version.
- Do not add third-party dependencies in this task.
