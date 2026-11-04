# M3-19d test report

Status: PASS (round 2)

## Gates
- go vet ./...: clean
- go test -race -coverprofile ./... (Windows): all ok; go-test-coverage total 95.4 % (>= 70), package gate PASS
- folderbrowse statement coverage: package covered (windows-only/unix-only files split by OS)
- golangci-lint run: 0 issues; golangci-lint fmt --diff: clean; go mod tidy -diff: clean
- Windows -race -count=3 folderbrowse (incl. TestHungRootBoundsGoroutinesAndTime): ok
- Docker golang:1.27.3 -race -count=3, -u 1000:1000: ok; as root (-u 0:0): ok

## Acceptance criteria (evidence = tests)
- New/options/defaults: TestOptionDefaults, RootID - met
- Roots availability bounded/joined: TestRootsReportsAvailableRoot, TestHungRootBoundsGoroutinesAndTime - met
- List traversal refusals, links, hidden/system (Check and List), reserved incl. link-resolved, cap, sort, unreadable counted: TestListRefusesBadPaths, TestHiddenAndSystemFoldersAreRefused, TestLinkIntoReservedDirectoryIsHidden, TestLinksInsideAreListedAndOutsideAreNot, TestJunctionIsNotListed, TestHiddenAttributeIsNotListed, TestReservedDirectoriesAreNotListed, TestListCapTruncates, TestUnreadable/DeniedDirectoryIsCountedNotNamed - met
- Check errors + cleaned path: TestCheckReturnsCleanedAbsolutePath, TestMissingRootIsUnavailable, TestUnknownRootIsInvalid - met
- No absolute path in errors: TestErrorsDoNotLeakTheRootPath - met
- ServiceUser: TestServiceUserNeverFails - met
- Cancellation: TestCancelledContext - met

## History
- Round 2: PASS
