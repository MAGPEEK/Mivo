# Release 0.1.0-alpha.1 – Docker image

Task: M1-23 | Date: 2026-10-30 | Publisher: mivo-release

## Approval

The user approved this push in chat on 2026-10-30. Question: "Darf mivo-release das Image nach
docker.io/magpeek/mivo-media-server pushen, mit den Tags 0.1.0-alpha.1 und alpha (kein latest),
für linux/amd64 und linux/arm64?" Answer: "Ja, so pushen".

The approval covers exactly: repository `docker.io/magpeek/mivo-media-server`, tags
`0.1.0-alpha.1` and `alpha`, platforms `linux/amd64` and `linux/arm64`.

## Result

| Item | Value |
|---|---|
| Repository | `docker.io/magpeek/mivo-media-server` |
| Source commit | `a05a2870c503a6ffbab401a1e98f48ec92ac203e` (clean tree, `origin/main`) |
| Tags | `0.1.0-alpha.1`, `alpha` (same digest); `latest` was NOT pushed (`imagetools inspect :latest` -> not found) |
| Index digest | `sha256:cabe7ef2ad3a8564637cd39d80686d702013b154596f40c98dcfb27f88acf552` |
| linux/amd64 manifest | `sha256:f7cb3269342017ead45bb52ac98a16d8972393c00e80ebe58a6f350596ce4b87`, 4 layers, 197.4 MiB compressed (206 956 964 B layers + 4 290 B config) |
| linux/arm64 manifest | `sha256:8479412bffbcee3e55808175713e2029d2f1f19c899164fc8e872039789070a8`, 4 layers, 192.0 MiB compressed (201 343 320 B layers + 4 279 B config) |
| linux/arm/v7 | not published: outside the approval (the Dockerfile has GOARM support, but it was not built or tested) |
| Extra index entries | two `unknown/unknown` attestation manifests (BuildKit provenance, one per platform); they are not platforms |

## OCI labels (identical on both platforms, read from the registry)

```
org.opencontainers.image.title       = Mivo
org.opencontainers.image.description = Open-source media server that plays practically any format
org.opencontainers.image.source      = https://github.com/MAGPEEK/Mivo
org.opencontainers.image.url         = https://github.com/MAGPEEK/Mivo
org.opencontainers.image.licenses    = GPL-3.0-or-later
org.opencontainers.image.version     = 0.1.0-alpha.1
org.opencontainers.image.revision    = a05a2870c503a6ffbab401a1e98f48ec92ac203e
```

Version and revision are set with `--build-arg VERSION` / `--build-arg REVISION`; no source change
was needed.

## Commands

PowerShell, PATH refreshed first. `$sp` is the session scratchpad.

```powershell
# clean source export (not the working tree)
git archive HEAD -o $sp\mivo-src.tar; tar -xf $sp\mivo-src.tar -C $sp\src

# dedicated builder (the default docker driver cannot push multi-platform manifests)
docker buildx create --name mivo-release --driver docker-container
docker buildx inspect mivo-release --bootstrap

# pre-push builds, loaded locally for the smoke test (run in $sp\src)
docker buildx build --builder mivo-release --platform linux/<amd64|arm64> `
  -f packaging/docker/Dockerfile --build-arg VERSION=0.1.0-alpha.1 `
  --build-arg REVISION=a05a2870c503a6ffbab401a1e98f48ec92ac203e `
  --load -t mivo-release-test:<amd64|arm64> .

# push (the layers were cached from the builds above)
docker buildx build --builder mivo-release --platform linux/amd64,linux/arm64 `
  -f packaging/docker/Dockerfile --build-arg VERSION=0.1.0-alpha.1 `
  --build-arg REVISION=a05a2870c503a6ffbab401a1e98f48ec92ac203e `
  -t docker.io/magpeek/mivo-media-server:0.1.0-alpha.1 `
  -t docker.io/magpeek/mivo-media-server:alpha --push .

# verification
docker buildx imagetools inspect docker.io/magpeek/mivo-media-server:0.1.0-alpha.1
docker buildx imagetools inspect docker.io/magpeek/mivo-media-server:alpha
docker buildx imagetools inspect docker.io/magpeek/mivo-media-server:latest   # expected: not found
docker buildx imagetools inspect docker.io/magpeek/mivo-media-server:0.1.0-alpha.1 `
  --format '{{json (index .Image "linux/arm64").Config.Labels}}'
docker run --rm docker.io/magpeek/mivo-media-server:0.1.0-alpha.1 --version
docker run --rm --platform linux/arm64 docker.io/magpeek/mivo-media-server:0.1.0-alpha.1 --version
```

Docker Hub login was done by the user beforehand; no credentials were read or handled.

## Verification summary

Before the push (local images built from the git archive export):
- amd64 `--version`: `Mivo 0.1.0-alpha.1 (go1.27.3)`.
- arm64 `--version` (QEMU emulation): `Mivo 0.1.0-alpha.1 (go1.27.3)`.
- amd64 container started: `GET /api/v1/health` -> `{"checks":[{"name":"database","status":"ok"}],"status":"ok"}`,
  Docker health status `healthy`, PID 1 runs as uid 1000 (`Config.User` is `1000:1000` on both images).

After the push (from the registry):
- `imagetools inspect :0.1.0-alpha.1` lists linux/amd64 and linux/arm64; `:alpha` has the same index digest.
- `:latest` does not exist.
- Labels present on both platform images (see above).
- `docker run --rm docker.io/magpeek/mivo-media-server:0.1.0-alpha.1 --version` after pulling
  prints `Mivo 0.1.0-alpha.1 (go1.27.3)`; the arm64 variant prints the same under emulation.

## Notes

- ADR 0002's rename checklist needs `docker.io/magpeek/mivo-media-server` added (not edited here).
- The release has no git tag `v0.1.0-alpha.1` yet; `git tag -l` is empty. The image is traceable
  through the revision label.
- Base images are pinned by tag only (`debian:stable-slim`); digest pinning is planned with CI.
- The local builder `mivo-release` and its build cache remain on the dev machine.
