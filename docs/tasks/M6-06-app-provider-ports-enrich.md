# M6-06 App: provider ports and enrichment of one item
Owner: mivo-backend
Depends on: M6-02, M3-06
Files/areas: `internal/app/enrich/**` (new), `internal/domain/errors.go` (add `ErrUnavailable`, `ErrRateLimited`)

## Goal
The use case "enrich one item": gather local sources, ask the providers behind ports, merge
with `metadata.Merge` respecting locks, select artwork, save. No network knowledge in `app`.

## Acceptance criteria
- [ ] Consumer-side ports (1-3 methods each): `MovieLookup` (`Search`, `Details`),
      `ShowLookup` (`Search`, `Details`, `SeasonDetails`), `MusicLookup`, `ArtworkDownloader`
      (`Fetch(ctx, ref) (Stored, error)`), `MetadataStore` (current record, locks, provider IDs,
      match state), `ProviderCache`.
- [ ] `Enricher.EnrichItem(ctx, itemID)`: if the item has a provider ID use it, else search and
      `match.Decide`; auto match writes state `matched`, no confident match writes `unmatched`;
      `disabled` items and libraries/providers switched off are skipped without any call.
- [ ] The result is merged as sources `[user-current, nfo, provider, filename]` through
      `metadata.Merge`; a locked field is unchanged (test per lockable field).
- [ ] Provider artwork is registered as `provider` rows; selection follows ADR 0027 §8 and
      respects artwork locks; failure to download one image does not fail the item.
- [ ] `ErrUnavailable`/`ErrRateLimited`/`ErrNotFound` from a port are returned as typed errors
      for the queue (M6-11) to retry; the item keeps its local data.
- [ ] Privacy test: the lookup request types carry no path, file name or library id (compile-
      level: the types have no such fields; reviewer-checkable).
- [ ] Tests: fakes only (in-memory ports), incl. offline fake, ambiguous match, locked title,
      show with seasons and episodes, disabled state.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Language comes from the library/item setting (ADR 0027). Keep functions ≤ 50 lines: split
`resolveMatch`, `mergeRecord`, `chooseArtwork`. Episode matching uses season/episode numbers
from placement (path), never from the provider.
