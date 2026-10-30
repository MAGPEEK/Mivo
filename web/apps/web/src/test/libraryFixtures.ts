import type {
  ItemPage,
  ItemSummary,
  Library,
  LibraryList,
  ScanCounts,
  ScanStatus,
} from "@mivo/api-client";

import { jsonResponse, type FakeRoute } from "./fakeApi";

/** Scan counts before the first scan: all zero. */
export const zeroScanCounts: ScanCounts = {
  added: 0,
  updated: 0,
  removed: 0,
  unchanged: 0,
  failed: 0,
  skipped: 0,
};

/** Status of a library that was never scanned. */
export const neverScanned: ScanStatus = { state: "idle", counts: zeroScanCounts };

/** A scan that is running. */
export const runningScan: ScanStatus = {
  state: "running",
  startedAt: "2026-10-30T18:04:05Z",
  counts: { ...zeroScanCounts, added: 1 },
};

/** A scan that finished with every kind of count set. */
export const finishedScan: ScanStatus = {
  state: "idle",
  startedAt: "2026-10-30T18:04:05Z",
  finishedAt: "2026-10-30T18:04:31Z",
  counts: { added: 3, updated: 1, removed: 2, unchanged: 29, failed: 1, skipped: 4 },
};

/** A scan that stopped because the library folder is missing. */
export const rootUnavailableScan: ScanStatus = {
  state: "failed",
  startedAt: "2026-10-30T18:04:05Z",
  finishedAt: "2026-10-30T18:04:06Z",
  counts: zeroScanCounts,
  errorCode: "root_unavailable",
};

/** A movies library with two items, never scanned. Override fields per test. */
export function fakeLibrary(overrides: Partial<Library> = {}): Library {
  return {
    id: "3f6c1d2a-9b8e-4f70-8a1c-2d3e4f5a6b7c",
    name: "Movies",
    kind: "movies",
    available: true,
    itemCount: 2,
    scan: neverScanned,
    ...overrides,
  };
}

/** A playable item of {@link fakeLibrary}: MKV with H.264 and AAC. Override fields per test. */
export function fakeItem(overrides: Partial<ItemSummary> = {}): ItemSummary {
  return {
    id: "8a9b0c1d-2e3f-4a5b-8c6d-7e8f9a0b1c2d",
    libraryId: fakeLibrary().id,
    title: "Big Buck Bunny",
    media: {
      available: true,
      container: "mkv",
      durationSeconds: 596.458,
      videoCodec: "h264",
      audioCodec: "aac",
      width: 1920,
      height: 1080,
      bitDepth: 8,
    },
    ...overrides,
  };
}

/** `count` distinct items titled "Item 1", "Item 2", … in `libraryId`. */
export function fakeItems(count: number, libraryId = fakeLibrary().id): ItemSummary[] {
  return Array.from({ length: count }, (_, index) =>
    fakeItem({ id: `item-${String(index + 1)}`, libraryId, title: `Item ${String(index + 1)}` }),
  );
}

/** Answers `GET /api/v1/libraries` with `libraries`. */
export function librariesRoute(libraries: readonly Library[]): FakeRoute {
  return {
    method: "GET",
    path: "/api/v1/libraries",
    respond: () => jsonResponse({ libraries: [...libraries] } satisfies LibraryList),
  };
}

/** Answers `GET /api/v1/libraries/{id}` for `library` with whatever `current` returns. */
export function libraryRoute(library: Library, current: () => Library = () => library): FakeRoute {
  return {
    method: "GET",
    path: `/api/v1/libraries/${library.id}`,
    respond: () => jsonResponse(current()),
  };
}

/**
 * Pages through `items` like the server: `limit` from the query, an opaque cursor that is
 * the index of the next item. The first page also answers requests without a cursor.
 */
export function itemsRoute(libraryId: string, items: readonly ItemSummary[]): FakeRoute {
  return {
    method: "GET",
    path: `/api/v1/libraries/${libraryId}/items`,
    respond: ({ url }) => {
      const limit = Number(url.searchParams.get("limit") ?? items.length);
      const start = Number(url.searchParams.get("cursor") ?? 0);
      const end = start + limit;
      const page: ItemPage = { items: items.slice(start, end) };
      return jsonResponse(end < items.length ? { ...page, nextCursor: String(end) } : page);
    },
  };
}
