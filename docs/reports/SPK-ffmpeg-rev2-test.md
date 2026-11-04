# SPK FFmpeg recipe revision 2 (9.0.2-static2) - test report

Status: **PASS** (test run 1 of max 3). Worktree agent-a05d66980fd145dfd, uncommitted changes.

| Gate | Result |
|---|---|
| `go test ./packaging/...` (recipecheck with `-count=1`) | ok, 4 packages |
| `golangci-lint run ./packaging/...` | 0 issues |
| shellcheck v0.11.0 `-x scripts/*.sh verify/*.sh` (Docker, `MSYS_NO_PATHCONV=1`) | no findings |
| hadolint of `ffmpeg/build/Dockerfile` | clean |
| bats `test/build` (spk-build-test:dev, incl. fetch/package-gates/build-scripts) | 90 cases, 0 not ok |
| independent `docker build --no-cache --target x86_64` of the recipe | ffmpeg f91238e8...825b, ffprobe 6fc10934...8ff1 = expected, reproduced |
| `verify.sh x86_64` with Go adapter tests (default) | 38/38 checks passed (encoder mjpeg, muxer image2pipe PASS) |
| `inspect-spk.sh --release` x86_64 / armv8 / armv7 | 93 checks, 0 failed, 2 warnings each (no GNU ABI note, musl static; expected) |
| SPK ffmpeg/ffprobe hashes vs `sources.lock` (all 3 SPKs) | identical |
| SPK-11 smoke x86_64 / armv8 / armv7 | 28 / 28 / 82 passed, 0 failed |
| ImageRenderer corpus tests against the built static x86_64 ffmpeg+ffprobe (golang:1.27.3 container) | TestRenderCorpusImages (9 subtests), TestRenderLibraryArtwork (5), TestProbeRefusesImageSequencePatterns PASS; ffmpeg reports `9.0.2-static2` |

Not run: Docker packaging of the server image, web gates, full-repo Go suite (out of scope; only packaging/FFmpeg recipe changed).
The armv8/armv7 FFmpeg hashes were only compared SPK vs lock, not rebuilt independently.

## History
- Round 1: PASS.
