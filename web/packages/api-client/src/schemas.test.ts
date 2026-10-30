import { describe, expect, expectTypeOf, it } from "vitest";

import {
  createApiClient,
  type ItemPage,
  type LibraryList,
  type PlaybackRequest,
  type PlaybackSession,
  type Problem,
} from "./index";
import { jsonResponse, respondWith, testBaseUrl } from "./test/fakeFetch";

const libraryId = "3f6c1d2a-9b8e-4f70-8a1c-2d3e4f5a6b7c";
const fileId = "1c2d3e4f-5a6b-4c7d-8e9f-0a1b2c3d4e5f";
const sessionId = "5d4c3b2a-1f0e-4d9c-8b7a-6f5e4d3c2b1a";
const problemContentType = "application/problem+json";

const libraries: LibraryList = {
  libraries: [
    {
      id: libraryId,
      name: "Movies",
      kind: "movies",
      available: true,
      itemCount: 1,
      scan: {
        state: "idle",
        counts: { added: 1, updated: 0, removed: 0, unchanged: 0, failed: 0, skipped: 0 },
      },
    },
  ],
};

const itemPage: ItemPage = {
  items: [
    {
      id: "8a9b0c1d-2e3f-4a5b-8c6d-7e8f9a0b1c2d",
      libraryId,
      title: "Big Buck Bunny",
      media: { available: true, container: "mkv", videoCodec: "h264", audioCodec: "aac" },
    },
  ],
  nextCursor: "next",
};

const playbackRequest: PlaybackRequest = {
  fileId,
  startPositionSeconds: 600,
  profile: {
    name: "chromium-desktop",
    directPlay: [{ container: "mp4", videoCodecs: ["h264"], audioCodecs: ["aac"] }],
    hls: { delivery: "mse", videoCodecs: ["h264"], audioCodecs: ["aac"] },
    codecLimits: [{ codec: "h264", maxBitDepth: 8 }],
  },
};

const session: PlaybackSession = {
  id: sessionId,
  fileId,
  state: "starting",
  mode: "transcode",
  delivery: "hls",
  streamUrl: `/api/v1/playback/sessions/${sessionId}/hls/index.m3u8`,
  startPositionSeconds: 600,
  sourceContainer: "mkv",
  targetContainer: "mp4",
  streams: [
    { kind: "video", sourceIndex: 0, action: "copy", sourceCodec: "h264", targetCodec: "h264" },
    {
      kind: "audio",
      sourceIndex: 1,
      action: "transcode",
      sourceCodec: "ac3",
      targetCodec: "aac",
      targetChannels: 2,
      encoder: "aac",
    },
  ],
  reasons: [{ code: "audio_codec_not_supported", scope: "audio" }],
  warnings: [],
};

const sessionLimit: Problem = {
  type: "about:blank",
  title: "Service Unavailable",
  status: 503,
  code: "session_limit_reached",
};

describe("video contract", () => {
  it("lists the libraries", async () => {
    const fake = respondWith(() => jsonResponse(libraries, 200));
    const client = createApiClient({ baseUrl: testBaseUrl, fetch: fake.fetch });

    const { data } = await client.GET("/api/v1/libraries");

    expect(data).toEqual(libraries);
    expectTypeOf(data).toEqualTypeOf<LibraryList | undefined>();
  });

  it("pages through library items with limit and cursor", async () => {
    const fake = respondWith(() => jsonResponse(itemPage, 200));
    const client = createApiClient({ baseUrl: testBaseUrl, fetch: fake.fetch });

    const { data } = await client.GET("/api/v1/libraries/{libraryId}/items", {
      params: { path: { libraryId }, query: { limit: 50, cursor: "abc" } },
    });

    expect(data).toEqual(itemPage);
    expectTypeOf(data).toEqualTypeOf<ItemPage | undefined>();
    expect(fake.requests[0]?.url).toBe(
      `${testBaseUrl}/api/v1/libraries/${libraryId}/items?limit=50&cursor=abc`,
    );
  });

  it("creates a playback session from a typed request", async () => {
    const fake = respondWith(() => jsonResponse(session, 201));
    const client = createApiClient({ baseUrl: testBaseUrl, fetch: fake.fetch });

    const { data } = await client.POST("/api/v1/playback/sessions", { body: playbackRequest });

    expect(data).toEqual(session);
    expectTypeOf(data).toEqualTypeOf<PlaybackSession | undefined>();
    const request = fake.requests[0];
    expect(request?.method).toBe("POST");
    expect(await request?.json()).toEqual(playbackRequest);
  });

  it("returns the problem of a reached session limit", async () => {
    const fake = respondWith(() => jsonResponse(sessionLimit, 503, problemContentType));
    const client = createApiClient({ baseUrl: testBaseUrl, fetch: fake.fetch });

    const { error, response } = await client.POST("/api/v1/playback/sessions", {
      body: playbackRequest,
    });

    expect(response.status).toBe(503);
    expect(error).toEqual(sessionLimit);
    expectTypeOf(error).toEqualTypeOf<Problem | undefined>();
  });

  it("stops a session with DELETE", async () => {
    const fake = respondWith(() => new Response(null, { status: 204 }));
    const client = createApiClient({ baseUrl: testBaseUrl, fetch: fake.fetch });

    const { response } = await client.DELETE("/api/v1/playback/sessions/{sessionId}", {
      params: { path: { sessionId } },
    });

    expect(response.status).toBe(204);
    expect(fake.requests[0]?.method).toBe("DELETE");
    expect(fake.requests[0]?.url).toBe(`${testBaseUrl}/api/v1/playback/sessions/${sessionId}`);
  });
});
