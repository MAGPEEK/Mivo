import { ApiError } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import {
  createFakeApiClient,
  createFakeServer,
  jsonResponse,
  problemResponse,
  type FakeRoute,
} from "../../../test/fakeApi";
import { createTrickplayApi } from "./trickplayApi";

const path = "/api/v1/media-files/f1/trickplay";
const accepted = 202;
const manifest = {
  tileWidth: 160,
  tileHeight: 90,
  columns: 10,
  rows: 10,
  intervalSeconds: 10,
  sheetCount: 1,
  sheetUrls: ["/api/v1/media-files/f1/trickplay/0.jpg"],
};

function apiWith(routes: readonly FakeRoute[]) {
  return createTrickplayApi(createFakeApiClient(createFakeServer(routes).fetch));
}

const signal = new AbortController().signal;

describe("createTrickplayApi", () => {
  it("returns the manifest", async () => {
    const api = apiWith([{ method: "GET", path, respond: () => jsonResponse(manifest) }]);

    expect(await api.getManifest("f1", signal)).toEqual(manifest);
  });

  it("answers undefined for a 404 and throws for other failures", async () => {
    const missing = apiWith([]);
    const broken = apiWith([
      { method: "GET", path, respond: () => problemResponse(500, "internal") },
    ]);

    expect(await missing.getManifest("f1", signal)).toBeUndefined();
    await expect(broken.getManifest("f1", signal)).rejects.toBeInstanceOf(ApiError);
  });

  it("takes the empty 202 of the generation request as success", async () => {
    const api = apiWith([
      { method: "POST", path, respond: () => new Response(null, { status: accepted }) },
    ]);

    await expect(api.requestGeneration("f1", signal)).resolves.toBeUndefined();
  });

  it("throws when the generation is refused", async () => {
    const api = apiWith([
      { method: "POST", path, respond: () => problemResponse(422, "unsupported") },
    ]);

    await expect(api.requestGeneration("f1", signal)).rejects.toBeInstanceOf(ApiError);
  });
});
