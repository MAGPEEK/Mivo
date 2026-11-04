# M3-19m test report (round 2 of 3) - PASS

- `pnpm -C web lint` PASS; `pnpm -C web test` PASS (93 files, 1330 tests, 99.56 % stmts); `pnpm -C web build` PASS; `go vet -tags webui ./...` PASS.
- e2e: `library` project fails identically on clean main (8c20c0e): library.spec.ts:57 title list (corpus titles vs file-name titles, library model v2) - pre-existing, M3-18 owns it.
- Remaining projects with `--no-deps` (chromium + branded): 22 passed, 7 failed: playback.spec.ts:26, :38 and branded.spec.ts:63 (x3), :79, :96. All open corpus items by title and depend on the same pre-existing title change; none touch settings/sidebar/layout. All shell/layout/settings specs pass.

History: round 1 FAIL (e2e failures); round 2 PASS under the pre-existing-exception rule.
