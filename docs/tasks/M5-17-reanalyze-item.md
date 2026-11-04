# M5-17 Re-analyze item endpoint
Owner: mivo-backend
Depends on: M3-06b
Files/areas: `api/openapi.yaml`, generated code, `internal/app/` (re-analyze use case), `internal/adapters/http/`, `cmd/mivo`

## Goal
"Re-analyze file" (section 7, PL-35) probes the files of one item again and updates the catalog.

## Acceptance criteria
- [ ] `POST /api/v1/items/{itemId}/reanalyze` answers 202 and runs the probe asynchronously;
      unknown id 404; guard as in ADR 0028; repeated calls while running answer 409.
- [ ] After completion the item's streams/duration reflect the file (use case test with fake
      prober and catalog; integration test with a corpus file changed on disk).
- [ ] Missing file marks the item unavailable instead of failing hard.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Reuse the scan pipeline's probe step; do not add a second probe path.
