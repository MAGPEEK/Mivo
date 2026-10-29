# Architecture Decision Records

MADR-style records. An accepted ADR is never rewritten. To change a decision, add a new ADR
that says `Supersedes: NNNN` and set the old one's status to `Superseded by NNNN`.
`Proposed` ADRs are binding for implementers until the owner accepts or rejects them.

| ADR | Title | Status |
|---|---|---|
| [0001](0001-license-gpl-3.md) | License: GPL-3.0-or-later | Accepted |
| [0002](0002-configurable-working-title.md) | "Mivo" is a working title with a single configurable source | Accepted |
| [0003](0003-go-backend-hexagonal-architecture.md) | Go backend with hexagonal layering | Accepted |
| [0004](0004-sqlite-database.md) | SQLite as the only database | Accepted |
| [0005](0005-react-vite-frontend.md) | React + TypeScript + Vite frontend in a pnpm monorepo | Accepted |
| [0006](0006-local-only-accounts.md) | Local-only user accounts | Accepted |
| [0007](0007-local-tv-pairing.md) | Local TV pairing (discovery + Quick Connect) | Accepted |
| [0008](0008-pwa-instead-of-native-mobile.md) | PWA instead of native mobile apps | Accepted |
| [0009](0009-tmdb-project-key.md) | Bundled TMDB project key, own key optional | Accepted |
| [0010](0010-outbound-connections.md) | No telemetry; update check is the only automatic outbound connection | Accepted |
| [0011](0011-design-direction-coral.md) | Design direction: Plex-like layout, coral accent | Accepted |
| [0012](0012-languages-en-de.md) | English base, complete German, more via Weblate | Accepted |
| [0013](0013-webos-6-minimum.md) | webOS 6.0 is the minimum TV platform | Accepted |
| [0014](0014-docker-first-nas-distribution.md) | Docker-first distribution for NAS | Accepted |
| [0015](0015-ffmpeg-external-process.md) | FFmpeg as external process; jellyfin-ffmpeg bundled | Accepted |
| [0016](0016-go-module-path.md) | Go module path `github.com/magpeek/mivo` | Proposed |
| [0017](0017-api-contract-spec-first.md) | Spec-first REST API with generated stubs and client | Proposed |
| [0018](0018-configuration-and-os-paths.md) | Configuration and OS-specific paths | Proposed |
| [0019](0019-quality-gates.md) | Quality gates and their enforcement | Proposed |
| [0020](0020-embedded-web-ui.md) | Embedding the web UI into the Go binary | Proposed |
| [0021](0021-generated-test-media-corpus.md) | Generated test media corpus | Proposed |

## Template

```
# NNNN Title

- Status: Proposed | Accepted | Superseded by NNNN
- Date: YYYY-MM-DD
- Supersedes / Refines: (optional)

## Context
## Decision
## Consequences
```
