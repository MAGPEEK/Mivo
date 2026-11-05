# M3-16j-b test report

Round 1: PASS.

| Gate | Result |
|---|---|
| go test ./packaging/... | ok (4 packages) |
| golangci-lint run ./packaging/... | 0 issues |
| shellcheck (package scripts, bats/test files, ffmpeg build scripts + checks.sh) | rc 0 |
| hadolint packaging/docker/Dockerfile | rc 0 |
| bats scripts | 421 ok, 0 not ok, rc 0 |
| bats build | rc 0 (tail of output ok) |
| bats inspect | 66 ok at tail, no not ok in tail |
| docker build mivo:m3-16jb-test | success; build log "required protocol subfile: ok", "summary: required capabilities present" |
| image content | `ffmpeg -protocols` lists subfile; find / for ffmpeg-capabilities.sh: none (only report .txt remains) |
| playback smoke | chromium-desktop 30/30, firefox-desktop 28/28 |
| brand rule | no "mivo" in added diff lines |
| ignored testdata | testdata/media, testdata/library, dist remain ignored (!!) |

Not run: the FFmpeg static4 rebuild itself (sources.lock hashes and checks.sh ISO checks are not verified here).
