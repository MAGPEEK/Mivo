import type { ItemDetail, MediaFile, MediaStream, Problem } from "@mivo/api-client";

// Fakes of the item feature. The shared helpers in src/test belong to the library UI task in
// this wave, so the item tests keep their server and fixtures here.

/** Answers the requests of the API client without a network; same shape as `fetch`. */
export type ItemServerFetch = (request: Request) => Promise<Response>;

const itemPathPattern = /^\/api\/v1\/items\/([^/]+)$/;

/** A stream for {@link mediaFile}; without an index it gets its position in the file. */
export type StreamFixture = Omit<MediaStream, "index"> & { readonly index?: number };

type StreamFields = Partial<MediaStream> & Pick<MediaStream, "kind">;

/** A stream fixture with every required field but the index; tests add what the case needs. */
export function stream(fields: StreamFields): StreamFixture {
  return { default: false, forced: false, attachedPicture: false, ...fields };
}

/** A complete stream for functions that take one stream; the index defaults to 0. */
export function mediaStream(fields: StreamFields): MediaStream {
  return { ...stream(fields), index: fields.index ?? 0 };
}

/** Fields of {@link mediaFile}: any file field, with stream fixtures. */
export type MediaFileFields = Omit<Partial<MediaFile>, "streams"> & {
  readonly streams?: readonly StreamFixture[];
};

/** A probed file of the given streams; indexes follow the array order unless given. */
export function mediaFile(fields: MediaFileFields = {}): MediaFile {
  const streams = (fields.streams ?? []).map((entry, position) => ({
    ...entry,
    index: entry.index ?? position,
  }));
  return {
    id: "file-1",
    fileName: "Big.Buck.Bunny.mkv",
    sizeBytes: 276_134_947,
    modifiedAt: "2026-10-01T09:30:00Z",
    available: true,
    ...fields,
    streams,
  };
}

/** The item of the plan's example: `MKV · HEVC 10-bit HDR10 · AC-3 5.1 · SRT`. */
export const hdrMovie: ItemDetail = {
  id: "item-1",
  libraryId: "library-1",
  title: "Big Buck Bunny",
  files: [
    mediaFile({
      container: "mkv",
      durationSeconds: 5423.042,
      streams: [
        stream({
          kind: "video",
          codec: "hevc",
          bitDepth: 10,
          hdr: "hdr10",
          width: 3840,
          height: 2160,
          default: true,
        }),
        stream({ kind: "audio", codec: "ac3", channels: 6, language: "ger", default: true }),
        stream({
          kind: "audio",
          codec: "aac",
          channels: 2,
          language: "eng",
          title: "Director's commentary",
        }),
        stream({ kind: "subtitle", codec: "subrip", language: "eng", forced: true }),
        stream({ kind: "subtitle", codec: "subrip" }),
      ],
    }),
  ],
};

/** An item whose file could not be probed. */
export const unreadableItem: ItemDetail = {
  id: "item-2",
  libraryId: "library-1",
  title: "broken clip",
  files: [
    mediaFile({
      id: "file-2",
      fileName: "broken clip.avi",
      sizeBytes: 512,
      available: false,
      errorCode: "probe_failed",
    }),
  ],
};

/** The problem the server answers for an unknown item. */
export const notFoundProblem: Problem = {
  type: "about:blank",
  title: "Not Found",
  status: 404,
  code: "not_found",
};

function json(body: unknown, status: number, contentType = "application/json"): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": contentType } });
}

/**
 * A server that knows `items` by ID and answers 404 problem details for every other request
 * (system info included; the sidebar then shows "Server unreachable", which these tests ignore).
 */
export function itemServer(items: readonly ItemDetail[]): ItemServerFetch {
  return (request) => {
    // The client percent-encodes the ID into the path; a real server decodes it the same way.
    const encodedItemId = itemPathPattern.exec(new URL(request.url).pathname)?.[1];
    const itemId = encodedItemId === undefined ? undefined : decodeURIComponent(encodedItemId);
    const item = items.find((candidate) => candidate.id === itemId);
    return Promise.resolve(
      item === undefined
        ? json(notFoundProblem, notFoundProblem.status, "application/problem+json")
        : json(item, 200),
    );
  };
}

/** Records the item requests (paths only), so a test can count how often the page asked. */
export function recordingItemRequests(fetch: ItemServerFetch): {
  readonly fetch: ItemServerFetch;
  readonly itemRequests: string[];
} {
  const itemRequests: string[] = [];
  return {
    itemRequests,
    fetch: (request) => {
      const path = new URL(request.url).pathname;
      if (itemPathPattern.test(path)) {
        itemRequests.push(path);
      }
      return fetch(request);
    },
  };
}
