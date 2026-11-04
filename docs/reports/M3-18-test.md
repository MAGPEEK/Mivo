# M3-18 test report (round 3)

Status: PASS

| Gate | Result |
| --- | --- |
| corpus `gen-testmedia -only "library-*"` | ok (manifest, DBS 131 episodes) |
| `pnpm -C web lint` | ok (prettier, tsc clean) |
| `pnpm -C web test` | ok, coverage 99.56 % statements |
| `pnpm -C web build` | ok |
| `go vet -tags webui ./...` | ok |
| `pnpm -C web e2e` x3 | 38 passed each (1.0 - 1.2 min), no flaky, cache/304 test green in all runs |
| CI=1, testdata/library moved away | exit 1, "No library corpus at ...manifest.json. Generate it with ..." |
| no CI, corpus moved away | exit 0, 7 skipped, 31 passed (library-v2 skipped) |
| corpus restored | yes |

Acceptance: all criteria met (typed-library config, skip message, movies/posters, shows/season/episode/play,
DT-24 131 episodes, cache/304, axe, gates). library-v2 spec runtime: well under 2 minutes (individual tests 0.8 - 4.6 s;
whole e2e suite about 70 s).

## History
- Round 2: FAIL (CI guard criterion unmet)
- Round 3: PASS
