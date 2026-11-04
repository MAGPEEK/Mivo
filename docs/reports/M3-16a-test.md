# M3-16a test report

Status: PASS (round 1)

| Gate | Result |
|---|---|
| go vet ./... | clean |
| go test -race -coverprofile ./... | all ok |
| go-test-coverage | PASS, total 94.6 % (>=70), package/domain thresholds satisfied; new disc.go funcs 100 % |
| golangci-lint run (incl. depguard) | 0 issues |
| golangci-lint fmt --diff | clean |
| go mod tidy -diff | clean |
| domain forbidden imports (go list -deps) | none |

Acceptance: Layout values + DiscClip/DiscTitle (media/disc.go) met; errors disc_encrypted/disc_unsupported/disc_no_title met; ReasonDiscSource "disc_source" in playback/plan.go met; naming.PlaceDisc (naming/disc.go) with tests met; table tests and coverage met; gates met.

History: round 1 PASS.
