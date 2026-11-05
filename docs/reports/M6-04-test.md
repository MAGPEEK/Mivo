# M6-04 test report (round 3 of 3)

status: PASS

| Gate | Result |
|---|---|
| go vet ./... | clean |
| go test -race netaccess/... + domain | ok |
| go test ./... | all ok |
| golangci-lint run / fmt --diff | 0 issues / no diff |
| go mod tidy -diff | no diff (no new deps) |
| go-test-coverage | PASS, total 94.6 %, package thresholds satisfied |
| Docker Linux go test -race -count=3 netaccess | ok (1.68s) |
| Domain imports (go list -deps) | no database/sql, net/http, os/exec, adapters, app |

Criteria: all met. Evidence by test: A->B->B and A->B->A credential header never reaches other host
(TestCredentialHeaderNeverReachesOtherHostOnMultiHopRedirect), TestRedirectHosts, LRU eviction
(TestEvictionPicksLeastRecentlyUsedIdleHost, TestIdleHostsAreEvicted..., TestBusyHostsAreNotEvicted), SSRF table
(TestIsPublic incl. 127.0.0.1, 10.0.0.1, 169.254.169.254, ::1, ::ffff:10.0.0.1; TestGuardedDialerRefusesBeforeDialing
incl. DNS-to-private), GET/HEAD-only retries (TestNonIdempotentMethodsAreNeverRetried, TestHeadIsRetried), limiter,
Retry-After, breaker, redaction tests present. Tests use httptest and injected clock; no real network or sleeping.

History: rounds 1 and 2 aborted; round 3 PASS.
