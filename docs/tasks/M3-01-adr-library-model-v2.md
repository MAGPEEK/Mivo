# M3-01 ADR 0027 "Library model v2"
Owner: mivo-architect (deliverable written with the milestone breakdown on 2026-11-04)
Depends on: none
Files/areas: `docs/adr/0027-library-model-v2.md`, `docs/adr/README.md` (index row)

## Goal
One decision record fixes the milestone-3 library model before any code changes: the item
hierarchy and identity, the name parsing rules including absolute numbering (UX DT-24), the
schema of migration `00003`, metadata sources with merge and field locks, Kodi NFO, local
artwork, the image endpoint (UX LB-44), scanning v2 with watcher and periodic scans (UX AD-23)
and the outline of the second wave. It leaves room for milestone 6 (provider IDs, field locks
of the edit dialog UX ED-02/ED-05/ED-07, provider artwork) and respects ADR 0022 (library roots
read-only, nothing user-owned yet) and the layer rules of ADR 0019.

## Acceptance criteria
- [x] `docs/adr/0027-library-model-v2.md` exists with Context, Decision, Consequences and
      Status `Proposed`; it names what it refines (ADR 0022, 0018, 0004, plan row "Bilder").
- [x] `docs/adr/README.md` lists 0027 as `Proposed`.
- [x] The ADR states the DT-24 rule (bare numbers are episode numbers, never split into season
      and episode) and the "Dragon Ball Super, 131 files → 1 show, 1 season, 131 episodes"
      regression.
- [x] The schema section holds `item_provider_ids`, `item_field_locks`, `artwork` with the
      `provider` and `upload` sources and `items.metadata_language`, so milestone 6 adds no
      rebuild of the core tables.
- [x] Open decisions OD-M3-1 … OD-M3-5 are listed with a recommendation.
- [ ] The owner accepts the ADR (status `Accepted`) or answers the open decisions; tasks that a
      different answer changes are named in `M3-00-overview.md` and are updated by the
      architect before they start.
- [ ] Gates: none (documentation only). `mivo-reviewer` checks the ADR against ADR 0010,
      0019, 0022 and `docs/design/ux-spec.md`.

## Notes
- Implementers who must deviate from the ADR report it in their REPORT; the orchestrator
  records the deviation as a "Notes (M3-xx)" section in ADR 0027. Do not edit the Decision
  section of an accepted ADR.
- Task files quote the ADR sections they implement; when the ADR changes before acceptance,
  re-check those tasks.
