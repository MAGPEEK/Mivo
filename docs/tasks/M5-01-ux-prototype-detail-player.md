# M5-01 Prototype: detail pages and player (finish)
Owner: mivo-ux
Depends on: none
Files/areas: `docs/design/prototypes/detail.html`, `docs/design/prototypes/README.md`

## Goal
`detail.html` (status "in progress") becomes a reviewable draft covering every detail and player
element of M5, so the owner can approve it and `mivo-frontend` can build from it.

## Acceptance criteria
- [ ] Movie, show, season (grid and list), episode, extra and a no-artwork variant are
      clickable: DT-01...03, 05, 06, 08, 09, 11, 20...22; split Play button with "Play from start".
- [ ] Item overflow menu shows exactly the M5 actions of ux-spec section 7.
- [ ] Player: full layout PL-01...05, mode badge in all three colours with overlay and the
      reason hints (PL-40...42), options panel with Auto play and speed (PL-14, 15), queue panel
      (PL-32), next-up card (PL-08), mini player (PL-38), media info dialog (PL-36), phone variant.
- [ ] Legend lists element ids and open questions with a default each; README status `draft`.
- [ ] Self-contained: opens from disk, no network; en/de toggle; no hard-coded product name
      outside one brand constant; contrast of text over backdrop checked (WCAG AA).
- [ ] Gates: none (docs only); owner approval recorded in README (`approved`) before M5-40 starts.

## Notes
Do not draw M4/M6/M7 sections (quality ladder, ratings, edit) except as hidden/disabled
placeholders marked with the milestone.
