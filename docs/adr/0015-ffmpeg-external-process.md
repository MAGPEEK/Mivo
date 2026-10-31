# 0015 FFmpeg runs as an external process; jellyfin-ffmpeg is the bundled build

- Status: Accepted
- Date: 2026-10-29
- Source: "play any format" section in `docs/plan.md`

## Context

Mivo must demux and decode practically every container and codec, including disc
structures and hardware encoders. FFmpeg covers this. Linking it would tie Mivo to cgo and
mix FFmpeg's build-time licensing into the binary.

## Decision

- `ffmpeg` and `ffprobe` are **external processes**, started with `os/exec.CommandContext`.
  Arguments are passed as a slice and never go through a shell. The process is killed when
  the context is cancelled. FFmpeg is never linked.
- The bundled build is **jellyfin-ffmpeg** (GPL, with libbluray, libdvdnav, libass and HW
  drivers). It ships in the Docker image and the Windows installer together with its license
  notices and a source reference. No Jellyfin server code is used (ADR 0001).
- Executable paths are configurable. A system FFmpeg works, possibly with fewer features.
  Capabilities (encoders, hwaccels) are detected at startup, never assumed.
- **Interim:** until the hardware-acceleration milestone (4) or the distribution milestone
  (9), the Docker image may use Debian's `ffmpeg` package. Switching to jellyfin-ffmpeg is
  part of those milestones, including a check that builds exist for every target
  architecture (armv7 in particular). If an architecture has no jellyfin-ffmpeg build, it
  falls back to Debian's package.

## Consequences

- A crashing FFmpeg cannot crash the server.
- Process start overhead per stream is accepted.
- The image is larger because of FFmpeg's dependencies.

## Notes (2026-10-31, from SPK-02)

- **Licence problem of jellyfin-ffmpeg's GPL build**: jellyfin-ffmpeg's `gpl` builds link
  `libfdk-aac`. jellyfin patches FFmpeg's `configure`
  (`debian/patches/0026-remove-fdk-aac-from-nonfree.patch`) so it no longer counts libfdk_aac as
  non-free; upstream FFmpeg refuses that combination without `--enable-nonfree`, because the
  FDK-AAC licence is not GPL-compatible. Such a binary is not redistributable under the GPL, so a
  GPL-3.0 project must not bundle it. This affects the plan above to switch the Docker image and
  the Windows installer to jellyfin-ffmpeg in milestone 4 or 9: that switch needs a jellyfin build
  without libfdk-aac, an own build, or another decision. The current Docker image is not affected
  (it uses Debian's FFmpeg).
- The Synology packages use an own static FFmpeg build instead (ADR 0026 Notes, tasks SPK-15 and
  SPK-16); that recipe is a candidate for the Docker image and Windows later.
- jellyfin-ffmpeg and BtbN builds are glibc-dynamic and need Linux ≥ 4.18, and neither has a
  32-bit ARM build, which also matters for the "check armv7" point above.
