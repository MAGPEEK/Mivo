# SPK-13 prerequisite "spk inspect" - test report

Round 1. Status: PASS. Worktree agent-a2c7b88eaca83586e, uncommitted changes in
packaging/synology/test/inspect/{inspect-spk.sh,inspect-spk.bats,fixture.bash,expected-members.txt}
and packaging/synology/toolkit/README.md.

## Gates

| Gate | Result |
|---|---|
| docker build spk-inspect:dev | ok |
| bats packaging/synology/test/inspect | 1..47, 47 ok, 0 not ok |
| shellcheck v0.11.0 -s sh inspect-spk.sh | exit 0, no findings |
| shellcheck inspect-spk.bats fixture.bash | exit 0, no findings |
| Go / web gates | not applicable (shell tooling only) |

## Real SPKs (read-only mount of C:\Users\stefa\Documents\Mivo-SPK-Test, alpha.2)

x86_64, armv8, armv7, each in test and --release mode: exit 0,
`inspect-spk: 71 checks, 0 failed, 2 warnings`. The two warnings are
`WARN bin/ffmpeg` and `WARN bin/ffprobe: no GNU ABI note, kernel floor not checked here`,
also in --release mode, as expected.

## Mutated copies (copies in the container, originals untouched; x86_64 SPK)

| Mutation | Exit | Finding |
|---|---|---|
| member `../evil` | 1 | unsafe members, nothing extracted: ../evil |
| absolute member `/tmp/absf` | 1 | unsafe members, nothing extracted: /tmp/absf |
| duplicate `INFO` appended | 1 | members differ: INFO(duplicate, 2 times) |
| truncated to 100000 bytes | 1 | tar cannot list the archive |
| symlink member `lnk` | 1 | unsafe members, nothing extracted: lnk (link or special file) |

## Backlog row `| spk inspect |` - items

- Base-256 sizes big-endian: bats 24, README states it.
- Listing validated before extraction (`..`, absolute, links, devices); extraction with
  `--no-same-owner --no-same-permissions` (inspect-spk.sh:387): bats 19-21, mutations above.
- ELF note walk with 8-byte alignment, build-id before ABI tag: bats 36; no ABI note on
  ffmpeg/ffprobe is WARN in both modes: bats 37, real SPKs.
- Duplicates under pattern rules, `..`, absolute: bats 20-22.
- `set -e` decision documented in the header (script keeps `set -u`, explains why) - accepted.
- Unextractable archive is exit 1: bats 23, truncated mutation.
- Nits: INT/TERM traps (lines 1025-1026, bats 46), `grep -qxF` for id/port, exact shebang
  (bats 40), no long lines (>110 chars: 0), missing bats cases (no SOURCE.txt 12, no licence 13,
  big-endian ELF 34, ELF type 35, --expected 14), README note-less binary sentence (README:437),
  tool list including find, du, cut, wc, tr, head and checked by require_tools (line 47).
  `stat` is not used by the script, so its omission is correct. `echo`: no occurrence found in
  the grep for the portability nit.

## Findings

None.

## History

- Round 1: PASS.
