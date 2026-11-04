# M6-08 Artwork fetcher: provider images into the data directory
Owner: mivo-media
Depends on: M6-04, M6-06, M3-09
Files/areas: `internal/adapters/metadata/artworkfetch/**` (new), `testdata/providers/images/**`

## Goal
Implement `ArtworkDownloader`: download a provider image once, validate it and store it under
`<data>/artwork/<tag>.<ext>` so the existing image endpoint serves it (ADR 0031 §11).

## Acceptance criteria
- [ ] Only hosts of ADR 0031 §1 (allow-list injected) are fetched; `https` only; redirects to
      other hosts refused except `*.archive.org` for Cover Art Archive.
- [ ] Content-type sniffed from bytes (jpeg/png/webp only), size ≤ 15 MiB, dimensions read
      with a decode-config check, others rejected with `domain.ErrInvalid`.
- [ ] Written atomically (temp file + rename) inside the data directory; path derived only from
      the tag, never from the URL; `Stored` returned with tag (domain `artwork.Tag`).
- [ ] Re-fetching the same ref is a no-op (file exists, tag equal).
- [ ] Deleting an item's provider artwork removes the files (used by "Remove match").
- [ ] Tests: fixtures served by `httptest`; cases: wrong type, oversized, truncated, redirect
      to foreign host, path-traversal-looking URL, concurrent same-ref downloads (race).
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
No user-supplied URLs here (that is the later edit dialog); the allow-list is why the SSRF
guard is a second line of defence, not the only one.
