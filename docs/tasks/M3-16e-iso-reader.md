# M3-16e Disc adapter: pure-Go ISO 9660/UDF reader
Owner: mivo-media
Depends on: M3-16a
Files/areas: `internal/adapters/disc/iso` (new package)

## Goal
Read the file tree of a DVD/Blu-ray ISO and open files as `io.ReaderAt` without mounting.

## Acceptance criteria
- [ ] Reads UDF 1.02-2.60 (with ISO 9660 bridge fallback): list directories, open file extents as `io.ReaderAt` with size; multi-extent files supported.
- [ ] Exposes the same file-tree view the title resolvers (M3-16c/d) need (an `fs.FS`-like interface), so one resolver serves folders and ISOs.
- [ ] Bounded memory (no whole-image reads), malformed images return errors, fuzz test over the descriptor parser.
- [ ] Tests: ISOs generated in the test with an in-process writer for a minimal tree (or committed tiny fixtures < 100 KB), table tests for extents.
- [ ] Gates: lint + layer rules + coverage thresholds pass; no new go.mod requirement unless a maintained BSD/MIT UDF library is accepted via ADR note.

## Notes
Largest task of the chain (about 400 lines): if it overruns, split read-only ISO 9660 first and UDF as M3-16e2. ISO support may slip to the release after folder support (ADR 0029).
