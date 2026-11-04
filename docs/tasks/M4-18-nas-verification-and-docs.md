# M4-18 Weak-NAS verification protocol, hardware passthrough docs, release notes
Owner: mivo-media
Depends on: M4-15
Files/areas: `docs/` (streaming guide, `docs/development.md` section), `packaging/docker/README.md`, `docs/tasks/SPK-12`-style test protocol

## Goal
The 0.1.0 claim "software transcode is safe on a DS925+" is checked on the device, and users know how to enable hardware acceleration where it exists.

## Acceptance criteria
- [ ] Protocol `docs/testing/m4-nas-protocol.md`: with 1 and 2 simultaneous 1080p H.264 transcodes and one burn-in, record CPU load, web-app responsiveness
      (page load < 2 s), speed ≥ 1.0x for the 720p tier and ≥ 1.0x for original-resolution 1080p, pause/resume visible in the sessions list; owner fills the result table.
- [ ] If 1080p software speed is < 1.0x on the DS925+, document the recommended default quality (720p) and open a BACKLOG row; do not change code in this task.
- [ ] Docker docs: `--device /dev/dri` for VAAPI/QSV, `--gpus all` with NVIDIA toolkit for NVENC, `group_add` for `render`, how to read the test result.
- [ ] Synology notes: no iGPU on the DS925+, "none found" is expected; Intel NAS models and the `/dev/dri` caveat of the SPK (no extra permissions) described as unverified.
- [ ] Release-note text collected for the next version (new defaults, subtitle behaviour, throttling Linux-only).
- [ ] Gates: markdown only; links checked.

## Notes
- Real measurements are by the owner (memory: owner NAS DS925+); this task writes the protocol and docs, the owner's run closes it.
