# M1-12 ui-tokens package: coral design tokens, Tailwind theme and brand.ts
Owner: mivo-frontend
Depends on: M1-11
Files/areas: `web/packages/ui-tokens/**`

## Goal
All colours, typography, spacing, radii and the product name come from one package that the
web app (and later the TV app) consumes. The coral identity from ADR 0011 is encoded and its
contrast is verified by tests.

## Acceptance criteria
- [ ] `src/colors.ts` exports the ADR 0011 values exactly: background `#0E1014`, surface
      `#181B21`, surfaceRaised `#22262E`, divider `#2C313A`, text `#E8EAED`, textMuted
      `#9AA0A6`, accent `#FF6B57`, accentHover `#FF8572`, onAccent `#3A0E07`. It also exports
      status colours `directPlay`, `directStream` and `transcode` (green, blue, amber; values
      in Notes).
- [ ] `src/typography.ts` exports the Inter font stack and size scales for `web` and
      `tenFoot`. No `tenFoot` size is below 20 px. `src/spacing.ts`, `src/radii.ts` (6, 8,
      10 px) and `src/shadows.ts` export named tokens. `src/index.ts` re-exports everything.
- [ ] `src/brand.ts` exports `productName = 'Mivo'`, `productId = 'mivo'` and
      `repositoryUrl`. A case-sensitive search for `Mivo` in `web/**/*.{ts,tsx}` (excluding
      `node_modules`) finds it only in `brand.ts` and its test.
- [ ] `src/theme.css` declares the tokens for Tailwind v4 in an `@theme` block (e.g.
      `--color-accent`, `--radius-md`, `--font-sans`). The package exports it as
      `@mivo/ui-tokens/theme.css`.
- [ ] Drift test: a Vitest test parses `theme.css` and asserts every colour token from
      `colors.ts` appears with the identical value, and the reverse.
- [ ] Contrast test using an exported, unit-tested `contrastRatio(foreground, background)`
      (WCAG 2.x formula): text and textMuted on background, surface and surfaceRaised are
      ≥ 4.5; onAccent on accent is ≥ 4.5; accent and every status colour on background and
      surface are ≥ 3.0.
- [ ] The package has no runtime dependencies.
- [ ] Tests: drift, contrast and `contrastRatio` edge cases (black/white = 21, identical = 1,
      3- and 6-digit hex, invalid input throws a typed error).
- [ ] Gates: `pnpm -C web lint` and `pnpm -C web test` (thresholds) pass.

## Notes
- Proposed status colours (adjust only if contrast fails, and report it): directPlay
  `#3DD68C`, directStream `#4C9AFF`, transcode `#F5B544`.
- Expected key contrast: onAccent on accent is about 6.0:1, muted text on background about
  7:1.
- This package is the only place where hex literals are allowed (lint rule from M1-11).
- Do not ship font files here. Self-hosting Inter happens in the app (M1-14, ADR 0010).
