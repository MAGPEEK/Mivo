# SPK-14 spkstage: stage the toolkit project
Owner: mivo-synology
Depends on: SPK-02, SPK-03, SPK-04, SPK-06, SPK-08
Files/areas: `packaging/synology/cmd/spkstage/**` (new Go command + tests)

## Goal
One Go command turns the repository's package templates and the prebuilt server binary and
FFmpeg of a family into a complete toolkit project that SPK-04's `run.sh` packs into the server
SPK without further edits.

## Acceptance criteria
- [ ] `go run ./packaging/synology/cmd/spkstage -family <x86_64|armv8|armv7> -version <semver>
      -binary <path> -ffmpeg-dir <dir> -out <dir> [-package-revision N] [-source-date-epoch N]`
      writes the project layout of `packaging/synology/toolkit/README.md` ("staged project"):
      `build.env` (SPK-03), `INFO.sh`, `SynoBuildConf/{depends,build,install}`, `scripts/` (the
      seven scripts plus `common.sh` and the rendered `package.env`), `conf/{privilege,resource}`,
      `WIZARD_UIFILES/`, `LICENSE` (repository), `PACKAGE_ICON.PNG` (64 × 64) and
      `PACKAGE_ICON_256.PNG` (256 × 256) from SPK-08, `prebuilt/bin/<id>`, `prebuilt/bin/ffmpeg`,
      `prebuilt/bin/ffprobe`, `prebuilt/share/licenses/ffmpeg/*` (licence files and `SOURCE.txt`
      from the FFmpeg directory), and the rendered `.sc` at the payload path that becomes
      `port_conf/<id>.sc` in `package.tgz` (coordinate the exact `prebuilt/` path with
      `SynoBuildConf/install`).
- [ ] The packed INFO of the toolkit check below has no `adminurl` line (aligned by the SPK-04
      rework; if it is not done yet, report it instead of editing `INFO.sh` here).
- [ ] Modes are set explicitly (executables and scripts 0755, everything else 0644, directories
      0755); every file gets the modification time of `-source-date-epoch`.
- [ ] Refusals (exit 1, message naming the input): unknown family; missing or non-executable inputs;
      a binary for the wrong ELF machine; leftover `@NAME@` tokens; non-ASCII bytes in
      `INFO.sh` and `SynoBuildConf/*` (scripts may carry UTF-8 messages, accepted in review), CR
      in every rendered text file; INFO-unsafe values (SPK-03); an existing non-empty project
      directory (unless `-force`). An FFmpeg `SOURCE.txt` with status `interim` is staged but
      reported as a warning on stderr.
- [ ] Go tests with a fake binary and fake FFmpeg files: the staged tree's file list and modes equal a
      golden list per family; the armv7 `build.env` has `ARCH="armv7 armada38x monaco"`; every
      JSON file parses; `build.env` has the expected `DSM_VERSION` and `BETA`; no staged file
      contains an unreplaced token; two runs give identical trees. `main` stays thin
      (`run(args, stdout, stderr) error`); coverage ≥ 80 % excluding `main`.
- [ ] Toolkit check: staging with the real linux/amd64 server binary (`GOOS=linux GOARCH=amd64
      CGO_ENABLED=0 go build -tags webui …` after `pnpm -C web build`, or without `webui` for this
      check) plus the interim x86_64 FFmpeg from `fetch.sh`, then `run.sh x86_64` → one SPK;
      `tar -tvf` output and INFO in the REPORT.
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, `go test ./...`,
      coverage gate, `go mod tidy -diff`; shellcheck, script tests (SPK-05/06) and
      `packaging/synology/toolkit/test.sh` still green.

## Notes
- Standard library only; reuse `internal/spkmeta` and `internal/spkicon`, no own rendering logic.
- One family per call; SPK-07 calls it three times.
- If SPK-12 later shows that DSM does not keep `common.sh`/`package.env` in `scripts/`, this
  command gets an option to inline them into every script (follow-up task).
- Docker Desktop shows bind-mounted files as 0777; modes in the staged tree carry no information on
  Windows hosts, which is why `SynoBuildConf/install` sets them again.

## Orchestrator note (from the SPK wave-2 review)
- package.env has one source of truth: `spkmeta.Package.PackageEnv()`. SPK-14 writes the file from
  it and deletes the `@TOKEN@` template `packaging/synology/package/scripts/package.env`; add a
  contract test that feeds spkmeta's output to `common.sh` (bats fixture or Go test).
- Extend the CRLF refusal in `toolkit/run.sh` to `scripts/*`, `conf/*` and the wizard `.sh` files
  (today only INFO.sh, build.env and SynoBuildConf/* are checked although README, ADR 0026 Notes and
  the SPK-05 notes claim more), with a test; accept only the 12 known build.env field names.
