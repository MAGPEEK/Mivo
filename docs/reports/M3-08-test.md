# M3-08 test report

Status: PASS (round 2 of max 3)

## Gates
- go vet ./...: clean
- go test -race -coverprofile ./...: all ok
- go-test-coverage: total 96.2 % (>= 70 %), PASS. Domain packages 100 %. image*.go: Render 93.8 %, rest 100 %.
- golangci-lint run --allow-parallel-runners: 0 issues
- golangci-lint fmt --diff: clean
- go mod tidy -diff: clean
- Image corpus tests, local FFmpeg 9.0.2: PASS (TestRenderCorpusImages 9 cases, TestRenderLibraryArtwork 5 cases, TestRenderRefusesAStreamThatIsNoPicture, TestImageRendererFailures 9 cases, cancellation, args)
- Same tests in Docker golang:1.27.3 with Debian FFmpeg 7.1.5: PASS, no skips

## Acceptance criteria
- Render scales, never upscales, JPEG/PNG(alpha), stream index: met (corpus: downscaled/never_upscaled for jpeg/png/webp, logo keeps alpha, mkv cover, mp4 covr)
- Safety flags (protocol_whitelist, max_pixels 64 MP, frames:v 1, pipe:1, map_metadata, % path refused): met (image_args.go:45-60, TestImageArguments*)
- Limits (8 MiB out, 15 s timeout, missing tool, undecodable, cancel): met (TestImageRendererFailures, ...ReportsCancellation)
- 32 MiB source check before start: met (image.go:27,136; "oversized source is refused before the tool starts")
- Unit/fake-tool/corpus tests: met
- Docker check: met
- Gates: met

## History
- Round 2: PASS
