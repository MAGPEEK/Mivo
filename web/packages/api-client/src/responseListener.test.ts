import { describe, expect, it, vi } from "vitest";

import { createApiClient } from "./client";
import { addResponseListener, type ObservedResponse } from "./responseListener";
import { jsonResponse, respondWith, testBaseUrl } from "./test/fakeFetch";

const unauthenticated = {
  type: "about:blank",
  title: "Unauthenticated",
  status: 401,
  code: "unauthenticated",
};

describe("addResponseListener", () => {
  it("reports method, contract path and response of every answer", async () => {
    const fake = respondWith(() => jsonResponse(unauthenticated, 401, "application/problem+json"));
    const client = createApiClient({ baseUrl: testBaseUrl, fetch: fake.fetch });
    const observed: ObservedResponse[] = [];
    addResponseListener(client, (event) => {
      observed.push(event);
    });

    const result = await client.GET("/api/v1/items/{itemId}", {
      params: { path: { itemId: "item-1" } },
    });

    expect(observed).toHaveLength(1);
    expect(observed[0]?.method).toBe("GET");
    expect(observed[0]?.schemaPath).toBe("/api/v1/items/{itemId}");
    expect(observed[0]?.response.status).toBe(401);
    // The caller still reads the body the listener did not consume.
    expect(result.error).toMatchObject({ code: "unauthenticated" });
  });

  it("keeps the answer when the listener fails", async () => {
    const fake = respondWith(() => jsonResponse({ required: true }, 200));
    const client = createApiClient({ baseUrl: testBaseUrl, fetch: fake.fetch });
    addResponseListener(client, () => Promise.reject(new Error("listener broke")));

    const result = await client.GET("/api/v1/setup");

    expect(result.data).toEqual({ required: true });
  });

  it("stops observing once removed", async () => {
    const fake = respondWith(() => jsonResponse({ required: false }, 200));
    const client = createApiClient({ baseUrl: testBaseUrl, fetch: fake.fetch });
    const listener = vi.fn();
    const remove = addResponseListener(client, listener);

    remove();
    await client.GET("/api/v1/setup");

    expect(listener).not.toHaveBeenCalled();
  });
});
