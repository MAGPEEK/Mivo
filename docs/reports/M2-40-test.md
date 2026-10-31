# M2-40 test report

Status: PASS (round 1)

## Gates
| Gate | Result |
|---|---|
| `go vet ./...` (windows and `GOOS=linux`) | clean |
| `go test -coverprofile ./...` | all ok; libraryfs 89.0 %, sqlite 94.8 %, app/scan 97.9 % |
| go-test-coverage | PASS, total 96.1 % (5861/6101), threshold 70 % |
| `golangci-lint run` | 0 issues; `GOOS=linux` for libraryfs and app/scan: 0 issues |
| `golangci-lint fmt --diff` | clean |
| `go mod tidy -diff`, `go -C tools mod tidy -diff` | clean |
| Docker `go test -race ./...` (golang:1.27.3) | all packages ok |
| Docker libraryfs `-v` | link tests ran on Linux and PASS (TestWalkSkipsLinkedDirectoriesWithAnUnknownIdentity, TestWalkListsALinkedDirectoryWithAKnownIdentityOnce, TestAdmitListsUnknownIdentitiesOnlyInTheRealTree, TestWalkTerminatesOnDirectorySymlinkLoops, TestWalkFollowsSymlinksThatStayInsideTheRoot, TestWalkListsEveryCrossLinkedDirectoryOnce). 3 permission tests SKIP (container runs as root), pre-existing. |

## Acceptance criteria
- Catalog contract in ports.go and sqlite doc comment: met (same order and rules).
- Fake enforces rules and records violations: met (`TestTheCatalogFakeEnforcesTheAdapterRules`, `TestTheCatalogFakeAcceptsValidSavesAndIgnoresForeignRemoves`).
- Each rule has a sqlite test: met with existing tests, no new case needed: `TestSaveProbedFileRejectsInvalidRecordsAndChangesNothing` (rows: stored file moved to another item/library, new file of a stored item in another library, unknown library), `TestSaveProbedFileOfAnotherFileAtAStoredPathIsAConflict`.
- `scan_canceled` whenever context is done: met (`TestAScanStoppedDuringACatalogCallIsCanceled`: FileStates and SaveProbedFile interrupted -> canceled, INFO finish line without cause; failure while alive -> catalog_failed, WARN with cause).
- libraryfs unknown link identity skipped, info log, reported skipped, real tree still listed, with 10 s timeout guard (walkTimeout): met.
- Link-reached duplicate with known identity: met (`TestWalkListsALinkedDirectoryWithAKnownIdentityOnce`).
- doc.go, Walk doc, maxDirectoryDepth comments updated: met.
- Nits: met. Windows fallback via injectable `identityWithFallback` (`TestIdentityWithFallback`); unix and test comments fixed.
- Tests green on Windows and in Docker: met.
- BACKLOG: only two rows touched (libraryfs row deleted, app/scan row cut to pruning, due M7): met.

## Observations (non-blocking)
- `fileIDInfoUnsupported` also accepts `ERROR_INVALID_FUNCTION` (STATUS_NOT_IMPLEMENTED), beyond the two codes named in the task; the task allows "what the code already classifies as such" and the comment documents it. Reviewer may confirm.
- No sqlite file changed beyond a doc comment, as expected.

## History
- Round 1: PASS.
