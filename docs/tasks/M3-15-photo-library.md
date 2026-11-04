# M3-15 Photo library: albums, photos and EXIF
Owner: mivo-media (image tooling), mivo-backend (catalog)
Depends on: M3-11b, **OD-M3-4** answered (tooling)
Files/areas: to be fixed per sub-task; expected: `internal/domain/{library,naming,photo}`,
`internal/app/scan` (photo pipeline), a new image/EXIF adapter (depends on OD-M3-4),
`internal/adapters/sqlite` (migration for photo fields), `api/openapi.yaml`,
`internal/platform/config`, `scripts/gen-testmedia`, Docker image and SPK packaging if a tool
is bundled

## Goal
A `photos` library lists folders as albums and photos with their capture date, camera and
location, with fast thumbnails for JPEG, PNG, WebP, GIF, HEIC/HEIF, AVIF, TIFF and RAW (plan
"Fotos"); short videos in photo folders play like other videos. Data for the timeline,
lightbox and EXIF panel of M5. Second wave; ADR 0027 §10.

## Acceptance criteria
- [ ] Config and domain accept `kind: photos`; contract adds `photos`, item kinds
      `photo_album|photo`, a photo detail block (taken at, camera make/model, lens, exposure,
      ISO, focal length, orientation, GPS when present, dimensions).
- [ ] Placement: each directory is a `photo_album` (nested albums follow the folder tree),
      each image a `photo`; videos in a photo library become `photo`-like items playable
      through the existing playback path.
- [ ] Metadata: EXIF/XMP via the tool chosen in OD-M3-4; capture date falls back to the file
      modification time; orientation applied in thumbnails.
- [ ] Thumbnails through the image endpoint of ADR 0027 §8 (photos become artwork of
      themselves); HEIC and RAW per the OD-M3-4 decision; unsupported formats listed with a
      clear "unsupported" state, never failing the scan.
- [ ] Corpus additions close the ADR 0021 gaps chosen by OD-M3-4 (EXIF in photos, HEIC) with
      generated or freely licensed files; integration test against the manifest; E2E: a photo
      album shows thumbnails.
- [ ] Gates: all Go and web gates of the touched areas; Docker image and SPK builds if a tool
      is bundled.

## Notes
- **Split before implementation** into one-sitting tasks M3-15a … once OD-M3-4 is answered;
  a new ADR records the tooling (supersedes the "Bilder" row of the plan for photos).
- No face recognition or AI tagging (plan: "Galerie ohne KI"); no phone upload.
- Photo libraries can hold tens of thousands of files: thumbnails are rendered on demand, not
  during the scan.
