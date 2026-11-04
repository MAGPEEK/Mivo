# M3-16j Packaging: verify FFmpeg capabilities in Docker and SPK builds
Owner: mivo-synology
Depends on: M3-16f
Files/areas: `packaging/docker`, `packaging/synology` (build scripts and smoke only), `docs/` notes

## Goal
Prove the bundled FFmpeg builds contain what disc and photo support needs, with no new libraries.

## Acceptance criteria
- [ ] Build step or smoke script fails if `ffmpeg -formats` lacks `concat`, `mpeg`, `mpegts`, or `-protocols` lacks `file`/`concat`; photo decoders `mjpeg`, `png`, `webp`, `gif`, `tiff` are present.
- [ ] Reports (does not fail) whether `hevc`/`heif`/`libdav1d` decode is available, written into the install log used by the capability probe docs.
- [ ] Docker image build and SPK build (SPK-15/16) unchanged otherwise; sizes compared against the last release and recorded in the task report.
- [ ] Gates: image build + `scripts/playback-smoke` with the disc case on the Docker image; DS925+ check recorded per the owner-device notes.

## Notes
This is the whole packaging impact of OD-M3-4/5 = no libvips, no exiftool, no libbluray/libdvdnav. Needs Docker and network approvals as usual.
