# 0003 Go backend with hexagonal (ports & adapters) layering

- Status: Accepted
- Date: 2026-10-29
- Source: decision table and architecture section in `docs/plan.md`

## Context

The server must run as one small static binary on weak ARM NAS (≥ 1 GB RAM, arm64/armv7) and
on Windows. The domain logic (playback decision engine, name parser, permissions) must be
testable without I/O.

## Decision

- Backend language: **Go** (current stable). One binary `mivo`, `CGO_ENABLED=0`.
- HTTP uses `net/http` with Go routing patterns and small middleware. No web framework.
- Layers and dependency direction: `domain` ← `app` ← `adapters` ← `cmd`.
  - `internal/domain`: pure business logic. Standard library only, no I/O packages
    (`os`, `os/exec`, `net`, `net/http`, `database/sql`, …).
  - `internal/app`: use cases. Declares the small ports (1–3 methods) it needs. Imports
    `domain` only.
  - `internal/adapters`: port implementations (http, sqlite, ffmpeg, fs, metadata, dlna, …).
  - `internal/platform`: cross-cutting infrastructure (config, logging, OS paths, brand,
    build info, Windows service). Imports nothing from `domain`, `app` or `adapters`.
    Only `adapters` and `cmd` may import it.
  - `cmd/mivo`: composition root. Wiring only.
- Constructor injection. No globals, no `init()` side effects, no singletons.
- Enforcement: depguard in golangci-lint plus an architecture test (ADR 0019).

## Consequences

- `app` gets configuration values through constructor options, never by importing
  `platform/config`.
- Each adapter can be swapped or faked. Use-case tests use in-memory fakes.
- Some duplication of small structs between layers (for example build info) is accepted to
  keep the direction clean.
