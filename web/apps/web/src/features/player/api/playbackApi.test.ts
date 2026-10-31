import { describe, expect, it } from "vitest";

import { createFakeApiClient, jsonResponse, problemResponse } from "../../../test/fakeApi";
import { PlaybackRequestError } from "../lib/playbackErrors";
import { createPlaybackServer, problemWithRetryAfter } from "../test/fakePlaybackServer";
import { fakeSession, mseProfile, playerFileId } from "../test/playerFixtures";
import { createPlaybackApi } from "./playbackApi";

const request = {
  fileId: playerFileId,
  profile: mseProfile,
  startPositionSeconds: 0,
  replacesSessionId: undefined,
};

function apiFor(server: { readonly fetch: (request: Request) => Promise<Response> }) {
  return createPlaybackApi(createFakeApiClient(server.fetch));
}

async function rejection(promise: Promise<unknown>): Promise<PlaybackRequestError> {
  const error: unknown = await promise.then(
    () => undefined,
    (reason: unknown) => reason,
  );
  if (!(error instanceof PlaybackRequestError)) {
    throw new Error("expected a PlaybackRequestError");
  }
  return error;
}

describe("createPlaybackApi createSession", () => {
  it("sends only the fields that are set", async () => {
    const server = createPlaybackServer();
    const api = apiFor(server);

    await api.createSession(request);
    await api.createSession({ ...request, startPositionSeconds: 12.5, replacesSessionId: "old" });

    expect(server.sessionRequests).toEqual([
      { fileId: playerFileId, profile: mseProfile },
      {
        fileId: playerFileId,
        profile: mseProfile,
        startPositionSeconds: 12.5,
        replacesSessionId: "old",
      },
    ]);
  });

  it("returns the created session", async () => {
    const session = await apiFor(createPlaybackServer()).createSession(request);

    expect(session).toEqual(fakeSession({ id: "session-1" }));
  });

  it("rejects with status, code and the Retry-After delay", async () => {
    const server = createPlaybackServer({
      createSession: () => problemWithRetryAfter(503, "session_limit_reached", "9"),
    });

    const error = await rejection(apiFor(server).createSession(request));

    expect(error).toMatchObject({
      status: 503,
      code: "session_limit_reached",
      retryAfterSeconds: 9,
    });
  });

  it("rejects without a delay when the server sends none", async () => {
    const server = createPlaybackServer({ createSession: () => problemResponse(422, "invalid") });

    const error = await rejection(apiFor(server).createSession(request));

    expect(error).toMatchObject({ status: 422, code: "invalid", retryAfterSeconds: undefined });
  });

  it.each([
    { name: "a failed connection", failure: new TypeError("Failed to fetch") },
    // The API client passes abort errors through unchanged; the engine never aborts, but a
    // rejection must still become a PlaybackRequestError.
    {
      name: "an aborted request",
      failure: Object.assign(new Error("Aborted"), { name: "AbortError" }),
    },
  ])("rejects $name as a network error", async ({ failure }) => {
    const api = apiFor({ fetch: () => Promise.reject(failure) });

    const error = await rejection(api.createSession(request));

    expect(error).toMatchObject({ status: 0, code: "network_error", retryAfterSeconds: undefined });
    expect(error.cause).toBeInstanceOf(Error);
  });
});

describe("createPlaybackApi getSession", () => {
  it("returns the session", async () => {
    const session = fakeSession({ id: "s", state: "starting" });
    const server = createPlaybackServer({ getSession: () => jsonResponse(session) });

    expect(await apiFor(server).getSession("s")).toEqual(session);
    expect(server.lookedUpSessions).toEqual(["s"]);
  });

  it("rejects with the status of a lost session", async () => {
    const error = await rejection(apiFor(createPlaybackServer()).getSession("gone"));

    expect(error).toMatchObject({ status: 404, code: "not_found" });
  });
});

describe("createPlaybackApi stopSession", () => {
  it("sends a DELETE that outlives the page", async () => {
    const server = createPlaybackServer();

    apiFor(server).stopSession("session-7");

    await expect.poll(() => server.stoppedSessions).toEqual(["session-7"]);
    expect(server.stopRequests[0]?.keepalive).toBe(true);
  });

  it("ignores a failed DELETE", async () => {
    let attempts = 0;
    const api = apiFor({
      fetch: () => {
        attempts += 1;
        return Promise.reject(new TypeError("offline"));
      },
    });

    api.stopSession("session-7");

    await expect.poll(() => attempts).toBe(1);
  });
});
