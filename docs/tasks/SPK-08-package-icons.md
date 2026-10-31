# SPK-08 Generated, name-free package icons
Owner: mivo-synology
Depends on: none (pixel sizes from SPK-01 if available, else 64 and 256)
Files/areas: `packaging/synology/internal/spkicon/**` (new Go package + tests)

## Goal
Package Center shows a recognisable icon in the project's coral accent, generated at build time
so the repository holds no binary assets and the icon contains no product name.

## Acceptance criteria
- [ ] A function renders a square PNG of a given size: rounded square in coral `#FF6B57` (ADR 0011)
      on a transparent background, with a centred play triangle in `#3A0E07` (text-on-accent);
      no text.
- [ ] Sizes used by the staging step (SPK-14) are named constants matching SPK-01 Q4 (default 64 for
      `PACKAGE_ICON.PNG`, 256 for `PACKAGE_ICON_256.PNG`); sizes outside 16–1024 are an error.
- [ ] Tests: decoded PNG has the requested dimensions; a corner pixel is fully transparent; the
      centre pixel has the triangle colour; a pixel between triangle and edge has the coral
      colour; two renders are byte-identical.
- [ ] Coverage of the package ≥ 85 %.
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, `go test ./...`,
      coverage gate.

## Notes
- Standard library only (`image`, `image/png`, `image/color`); anti-aliasing is welcome but
  optional.
- This is a placeholder until the project has a logo; then the logo replaces the generator (the
  owner or `mivo-frontend` supplies it). Keep the colours as named constants with a comment
  pointing to ADR 0011 and `web/packages/ui-tokens`.
