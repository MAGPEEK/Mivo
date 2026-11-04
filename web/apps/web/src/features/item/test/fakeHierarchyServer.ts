import type { ItemAncestor, ItemDetail, ItemPage, ItemSummary } from "@mivo/api-client";

import {
  createFakeServer,
  jsonResponse,
  problemResponse,
  type FakeServer,
} from "../../../test/fakeApi";
import { mediaFile } from "./fakeItemServer";

// Fixtures of a show with seasons and episodes and a fake server that pages their children.

const libraryId = "library-1";
const notFoundStatus = 404;

/** A show with two seasons (the specials last) but nothing to play. */
export const showWithoutPlayDetail: ItemDetail = {
  id: "show-1",
  libraryId,
  kind: "show",
  title: "The Test Show",
  year: 2008,
  summary: "A chemistry teacher tests things.",
  genres: ["Drama"],
  artwork: { poster: { id: "show-poster", tag: "tag-1" } },
  childCount: 2,
  leafCount: 12,
  files: [],
};

/** The show of the fixtures: two seasons, the specials last. */
export const showDetail: ItemDetail = { ...showWithoutPlayDetail, playItemId: "episode-1" };

const showAncestor: ItemAncestor = { id: "show-1", kind: "show", title: "The Test Show" };

/** Season 1 (first, so it has no previous season). */
export const seasonOneDetail: ItemDetail = {
  id: "season-1",
  libraryId,
  kind: "season",
  title: "Season 1",
  childCount: 9,
  playItemId: "episode-1",
  nextId: "season-0",
  ancestors: [showAncestor],
  files: [],
};

/** The specials (season 0), the last one. */
export const specialsDetail: ItemDetail = {
  id: "season-0",
  libraryId,
  kind: "season",
  title: "Specials",
  childCount: 3,
  previousId: "season-1",
  ancestors: [showAncestor],
  files: [],
};

/** A season summary as the children endpoint of the show lists it. */
export function seasonSummary(number: number, childCount: number): ItemSummary {
  return {
    id: `season-${String(number)}`,
    libraryId,
    kind: "season",
    title: number === 0 ? "Specials" : `Season ${String(number)}`,
    number,
    childCount,
    parentId: "show-1",
  };
}

/** An episode summary as the children endpoint of a season lists it. */
export function episodeSummary(
  number: number | undefined,
  fields: Partial<ItemSummary> = {},
): ItemSummary {
  return {
    id: `episode-${String(number ?? "x")}`,
    libraryId,
    kind: "episode",
    title: `Episode title ${String(number ?? "x")}`,
    parentId: "season-1",
    ...(number === undefined ? {} : { number }),
    ...fields,
  };
}

/** The detail of an episode of season 1. */
export function episodeDetail(id: string, fields: Partial<ItemDetail> = {}): ItemDetail {
  return {
    id,
    libraryId,
    kind: "episode",
    title: "Pilot",
    ancestors: [showAncestor, { id: "season-1", kind: "season", title: "Season 1", number: 1 }],
    playItemId: id,
    files: [
      mediaFile({ id: "episode-file", durationSeconds: 2700, container: "mkv", streams: [] }),
    ],
    ...fields,
  };
}

/** What a hierarchy server knows: item details and the children of containers, in order. */
export interface HierarchyData {
  readonly details: readonly ItemDetail[];
  readonly children: Readonly<Record<string, readonly ItemSummary[]>>;
}

/** A fake server with the item and children endpoints; children page by `limit` and `cursor`. */
export function hierarchyServer(data: HierarchyData): FakeServer {
  return createFakeServer([
    {
      method: "GET",
      path: "/api/v1/items/:itemId",
      respond: ({ params }) => {
        const item = data.details.find((candidate) => candidate.id === params["itemId"]);
        return item === undefined
          ? problemResponse(notFoundStatus, "not_found")
          : jsonResponse(item);
      },
    },
    {
      method: "GET",
      path: "/api/v1/items/:itemId/children",
      respond: ({ params, url }) => {
        const all = data.children[params["itemId"] ?? ""];
        if (all === undefined) {
          return problemResponse(notFoundStatus, "not_found");
        }
        const ordered = url.searchParams.get("order") === "desc" ? [...all].reverse() : all;
        const limit = Number(url.searchParams.get("limit") ?? ordered.length);
        const start = Number(url.searchParams.get("cursor") ?? 0);
        const page: ItemPage = {
          items: ordered.slice(start, start + limit),
          ...(start + limit < ordered.length ? { nextCursor: String(start + limit) } : {}),
        };
        return jsonResponse(page);
      },
    },
  ]);
}
