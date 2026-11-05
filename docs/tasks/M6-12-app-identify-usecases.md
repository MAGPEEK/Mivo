# M6-12 App: identify, unmatch, refresh use cases
Owner: mivo-backend
Depends on: M6-06, M6-11
Files/areas: `internal/app/identify/**` (new)

## Goal
The use cases behind the "Zuordnung korrigieren" dialog.

## Acceptance criteria
- [ ] `Search(ctx, itemID, query)`: query text, year, or a pasted ID/URL (`match.ParseProviderRef`);
      returns ranked candidates with confidence for the item's kind (movie → movie search,
      show/season/episode → show search at show level, album → music).
- [ ] `Apply(ctx, itemID, provider, externalID)`: stores the provider ID, locks
      `provider_ids`, sets `matched`, enqueues enrichment (reason `identify`) for the item and,
      for a show, its seasons and episodes; other locks untouched; previous provider data is
      replaced except locked fields.
- [ ] `Unmatch(ctx, itemID)`: state `disabled`, the `provider_ids` lock (the identify choice)
      released and provider IDs reset to the NFO ones (NFO IDs stay after the next scan, the
      item stays `disabled` per ADR 0031 §9, test documents this), provider artwork rows and
      files deleted, local metadata remains.
- [ ] `Refresh(ctx, itemID|libraryID)`: enqueues; refuses with `ErrConflict` when online
      metadata is disabled.
- [ ] Episode/season items resolve to their show for matching; applying on an episode is
      rejected with `domain.ErrInvalid`.
- [ ] Tests with fakes: re-identify wrong match keeps user locks, unmatch then rescan stays
      `disabled`, offline search returns `ErrUnavailable`.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Authorisation is the HTTP guard's job (ADR 0028); use cases stay user-agnostic until M7.

Apply asks no provider, so it works offline. The earlier provider artwork stays until the
queued enrichment replaces it (offline: until the provider is reachable again); Apply does not
delete it because a locked artwork kind may have selected one of those pictures.
