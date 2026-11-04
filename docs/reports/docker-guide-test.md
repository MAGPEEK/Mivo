# Test report: docker-guide (Docker and Compose guides)

Round 1, 2026-11-04. Result: **PASS**

Scope (uncommitted): docs/guides/docker.md, docs/guides/docker-compose.md,
packaging/docker/docker-compose.yml, README.md, docs/development.md, packaging/docker/README.md.
Image under test: `docker.io/magpeek/mivo-media-server:0.1.0-alpha.2` (already present locally,
`docker pull` reported "Image is up to date"; no login, no push). Host: Windows 11, Docker Desktop
29.9.2, Compose v5.6.1.

## Results

| # | Check | Result | Evidence |
|---|---|---|---|
| 1 | Relative links and anchors | met | Script over all 6 files: 26 relative links checked, 0 missing files, 0 broken anchors (GitHub slug rules) |
| 2 | `docker compose -f packaging/docker/docker-compose.yml config` | met | exit 0, resolved file printed (image, `stop_grace_period: 20s`, 3 bind mounts, `0.0.0.0:8460`) |
| 3 | Compose guide run | met | see below |
| 4 | `docker run` guide run | met | see below |
| 5 | No CR; no user-facing `mivo:dev` | met | 0 CR in all 6 files; remaining `mivo:dev` only in developer build sections (below) |
| 6 | Unchanged gates | met | `golangci-lint run`: 0 issues; `go test ./...`: all ok, no FAIL; `pnpm -C web lint`: exit 0 (eslint, dependency-cruiser, prettier, tsc) |

### 3. Compose guide (temp dirs, `testdata/media` already present)
Copy of the compose file with the three paths replaced, then `docker compose config`, `up -d`:
- `docker compose ps`: `Up 12 seconds (healthy)`, `0.0.0.0:8460->8460/tcp`.
- Log order as documented: `starting Mivo`, migrations, `ffmpeg ready ...`, `libraries synced ... libraries=1`,
  `http server listening address=[::]:8460 lan_only=true`, `library scan finished ... added=28 ... failed=0`.
- `/api/v1/system/info`: `{"apiVersion":"1","productName":"Mivo","serverId":"...","version":"0.1.0-alpha.2"}`.
- `/api/v1/libraries`: library `media`, `itemCount: 28`.
- `docker compose up -d` unchanged: "Container mivo Running" (idempotent). `stop`: 0.65 s with graceful
  log lines ending `Mivo stopped`. `down`: container and network removed, `config/{data,cache}` kept.

Permission failure (config owned by uid 2000, mode 700, container `--user 1000:1000`):
- Log: `mivo: load configuration: read /config/config.yaml: open /config/config.yaml: permission denied`;
  container exited with code 1. Matches the line quoted in docker.md section 10 and docker-compose.md
  (Synology section) character for character.
- Also verified the other two quoted messages: `/transcode` not writable gives
  `level=WARN msg="leftover playback session directories could not be deleted" error="... open /transcode: permission denied"`
  (container stays healthy); unreadable library root gives `level=WARN msg="library scan finished" ...
  state=failed ... code=root_unavailable cause="library folder is missing or unreadable"`.
- Deviation: the Docker Desktop bind mounts on Windows are mode 777 and ignore host ownership, so the
  failure was reproduced with named volumes chowned to 2000 (same kernel permission check). Note for
  anyone repeating this: an empty volume mounted at an image path (`/media`, `/transcode`) takes the
  image directory's ownership on first mount, so chown it first via a helper container and mount
  `/media` sub-paths for the root_unavailable case.

### 4. Plain `docker run` guide
- `docker pull` ok; `docker run --rm <image> --version` prints `Mivo 0.1.0-alpha.2 (go1.27.3)` as documented.
- Section 3 command run literally (`--user 1026:100`, `--stop-timeout 20`, `--restart unless-stopped`,
  `-e TZ`, three mounts): `Up 12 seconds (healthy)`, scan `added=28 failed=0`, system/info version
  `0.1.0-alpha.2`, `/` answers 200, `docker stop` 0.77 s with clean shutdown log, `docker start` healthy again.
- Observation (not a defect): the first attempt failed with `lock data directory ... /config/data: permission denied`
  because `config/data` from the earlier compose run (uid 1000, mode 750) was still in the same temp dir and the run
  used uid 1026. This is the situation the guide's Synology paragraph describes (remove the contents, start again);
  after clearing the folders the run succeeded.

### 5. `mivo:dev` occurrences
Only in docs/development.md lines 467-468, 489-490 (build-it-yourself and release-gate smoke instructions) and
packaging/docker/README.md lines 13, 20, 143 (line 13 explicitly says to substitute it for a self-built image;
20 and 143 are the build and smoke steps). All developer-facing; the user guides and compose file use only the
published tag.

### Cleanup
Containers `mivo`, `qa-perm`, network `mivo_default`, volumes `qa-cfg`, `qa-cfg2`, `qa-tr`, `qa-med` and the temp
directory were removed. Pre-existing containers (`reverent_shockley`, buildx) and other volumes belong to other work and
were left alone. `git status` shows only the expected modified/new files (plus `docs/adr/0002-...` and
`packaging/synology/README.md`, which belong to other work and were not part of this check).

## History
- Round 1: PASS.

REPORT
status: PASS
task: docker-guide
files: docs/reports/docker-guide-test.md
tests: link/anchor script 26 links 0 broken; docker compose config ok; compose and docker run guides executed against 0.1.0-alpha.2 (healthy, version 0.1.0-alpha.2, 28 items, quoted log lines match); golangci-lint 0 issues; go test ./... ok; pnpm -C web lint ok
findings: none blocking; note that docs/adr/0002-configurable-working-title.md and packaging/synology/README.md are also modified in the checkout but were outside this scope
next: proceed to review
