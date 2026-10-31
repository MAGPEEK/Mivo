# SPK-14 spkstage: stage the toolkit project
Owner: mivo-synology
Depends on: SPK-02, SPK-03, SPK-04, SPK-06, SPK-08
Files/areas: `packaging/synology/cmd/spkstage/**` (new Go command + tests)

## Goal
One Go command turns the repository's package templates and the prebuilt server binary and
FFmpeg of a family into a complete `source/<project>/` directory that `PkgCreate.py` packs into
the server SPK without further edits.

## Acceptance criteria
- [ ] `go run ./packaging/synology/cmd/spkstage -family <x86_64|armv8|armv7> -version <semver>
      -binary <path> -ffmpeg-dir <dir> -out <dir> [-package-revision N] [-source-date-epoch N]`
      writes `<out>/<id>/` with: rendered templates of `packaging/synology/package/`
      (`SynoBuildConf/`, `INFO.sh`, `scripts/`, `conf/`, `WIZARD_UIFILES/`, the `.sc` file),
      `build.env` and `package.env` (SPK-03), `PACKAGE_ICON.PNG` and `PACKAGE_ICON_256.PNG`
      (SPK-08), the repository `LICENSE`, the server binary, `ffmpeg`, `ffprobe`, the FFmpeg
      licence files and a source notice built from `sources.lock` (version, URLs, SHA-256), in the
      places `SynoBuildConf/install` expects (SPK-04).
- [ ] Modes are set explicitly (executables and scripts 0755, everything else 0644, directories
      0755) and every file gets the same modification time from `-source-date-epoch`, independent
      of the host file system.
- [ ] Errors (exit 1, message naming the input) for: unknown family, missing or non-executable
      inputs, a binary for the wrong machine (ELF header check), leftover `@NAME@` tokens, an
      existing non-empty `<out>/<id>/` (no silent overwrite unless `-force`).
- [ ] Go tests with a fake binary and fake FFmpeg files: the staged tree's file list and modes
      equal a golden list; every JSON file parses; `build.env` holds the expected `ARCH`,
      `DSM_VERSION`, `BETA`; no staged file contains an unreplaced token; two runs give identical
      trees (content and modes). `main` stays thin (`run(args, stdout, stderr) error`); coverage of
      the command ≥ 80 % excluding `main`.
- [ ] Toolkit check: staging with the real linux/amd64 server binary (`go build` with
      `GOOS=linux GOARCH=amd64 CGO_ENABLED=0`) plus SPK-02's x86_64 FFmpeg, then SPK-04's run
      script for x86_64 → one SPK; `tar -tvf` output and INFO in the REPORT.
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, `go test ./...`,
      coverage gate, `go mod tidy -diff`; shellcheck and script tests of SPK-05/06 still green.

## Notes
- Standard library only; reuse `internal/spkmeta` and `internal/spkicon`, no own rendering logic.
- The command stages exactly one family per call; SPK-07 calls it three times.
- What is staged is the server package; descriptions and wizard texts speak of the media server
  running on the NAS.
