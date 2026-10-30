# M1-01 Test Report – GPL-3.0 LICENSE file

Status: **PASS** (round 1)

Scope: no code involved, so no Go/web gates were run (per task: "Gates: none").

## Acceptance criteria

| # | Criterion | Result | Evidence |
|---|-----------|--------|----------|
| 1 | User approval for the download was obtained through the orchestrator before network access | Met | Orchestrator message to the tester: approval GRANTED by the project owner on 2026-10-30. The tester's independent download relied on that approval. |
| 2a | `LICENSE` at repo root, 674 lines | Met | `(Get-Content LICENSE).Count` = `674`; `wc -l` = `674` |
| 2b | Contains `GNU GENERAL PUBLIC LICENSE` and `Version 3, 29 June 2007` | Met | `LICENSE:1: GNU GENERAL PUBLIC LICENSE`, `LICENSE:2: Version 3, 29 June 2007` |
| 2c | Last non-empty line ends with `<https://www.gnu.org/licenses/why-not-lgpl.html>.` | Met | `tail -3` shows `<https://www.gnu.org/licenses/why-not-lgpl.html>.$` |
| 2d | Verbatim downloaded text | Met | SHA256 identical to independent download (see criterion 4) |
| 3 | `git ls-files --eol LICENSE` shows `i/lf` after staging | Met | Checked with a throwaway index (`GIT_INDEX_FILE` in scratchpad), so the real index stays untouched: `i/lf    w/lf    attr/text=auto eol=lf    LICENSE`. No CR bytes in the file (count 0). |
| 4 | SHA256 reported and compared with an independent download | Met | See below |
| 5 | Gates: none | N/A | No code |

### SHA256 comparison

```
implementer-reported  3972DC9744F6499F0F9B2DBF76696F2AE7AD8AF9B23DDE66D6AF86C9DFB36986
repo LICENSE          3972DC9744F6499F0F9B2DBF76696F2AE7AD8AF9B23DDE66D6AF86C9DFB36986
independent download  3972DC9744F6499F0F9B2DBF76696F2AE7AD8AF9B23DDE66D6AF86C9DFB36986
```

The independent download of `https://www.gnu.org/licenses/gpl-3.0.txt` went to a scratchpad directory
outside the repo. It has 674 lines and 0 CR bytes, identical to `LICENSE`.

## Other checks

- `README.md:21` states "GPL-3.0-or-later. See [LICENSE](LICENSE)." This is consistent with the file.
- `git diff --cached --name-only` is empty, so nothing is staged.
- `git status --short` shows `?? LICENSE` and `?? go.mod`. `go.mod` belongs to M1-02 (parallel work), not this task.
- No copyright line was added or text retitled: the first lines are the standard GPL header.


## History

- Round 1: PASS.
