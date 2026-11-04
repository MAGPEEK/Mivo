# M3-15a test report (round 1): FAIL

- go vet ./...: clean. go test -race ./...: all ok. go mod tidy -diff: clean. fmt --diff: clean.
- Coverage: total 94.7 % (go-test-coverage PASS); new photo/library/naming code >= 90 %.
- Domain forbidden imports (go list -deps): none.
- golangci-lint run (full): FAIL, 5x exhaustive (new enum values not handled in outer layers):
  - internal/adapters/http/restapi/items_detail_response.go:54 (ItemKindPhotoAlbum, ItemKindPhoto)
  - internal/adapters/http/restapi/library_response.go:77 (KindPhotos)
  - internal/app/scan/artwork.go:52 (ItemKindPhotoAlbum, ItemKindPhoto)
  - internal/app/scan/metadata.go:164 and :286 (ItemKindPhotoAlbum, ItemKindPhoto)
- Acceptance criteria: functional criteria met (tests green); "lint passes" NOT met.
- Fix: add explicit cases (e.g. return a not-yet-supported error / no-op with task ref) in those switches.
