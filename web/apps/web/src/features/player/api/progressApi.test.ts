import { describe, expect, it } from "vitest";

import { createFakeApiClient, problemResponse } from "../../../test/fakeApi";
import { createPlaybackServer } from "../test/fakePlaybackServer";
import { playerItemId } from "../test/playerFixtures";
import { createProgressSender, ProgressReportError } from "./progressApi";

function senderFor(reportProgress?: () => Response | Promise<Response>) {
  const server = createPlaybackServer(reportProgress === undefined ? {} : { reportProgress });
  return { server, send: createProgressSender(createFakeApiClient(server.fetch), playerItemId) };
}

describe("createProgressSender", () => {
  it("puts the report to the item's progress with keepalive", async () => {
    const { server, send } = senderFor();

    await send({ positionSeconds: 2530.5, playbackSessionId: "session-1" });

    expect(server.progressReports).toEqual([
      { positionSeconds: 2530.5, playbackSessionId: "session-1" },
    ]);
    const request = server.progressRequests[0];
    expect(request?.method).toBe("PUT");
    expect(new URL(request?.url ?? "").pathname).toBe(`/api/v1/items/${playerItemId}/progress`);
    expect(request?.keepalive).toBe(true);
  });

  it.each([
    [501, "not_implemented"],
    [401, "unauthenticated"],
    [403, "profile_required"],
  ])("stays silent for %i", async (status, code) => {
    const { send } = senderFor(() => problemResponse(status, code));

    await expect(send({ positionSeconds: 1 })).resolves.toBeUndefined();
  });

  it("rejects for an error another report may not get", async () => {
    const { send } = senderFor(() => problemResponse(503, "unavailable"));

    const error: unknown = await send({ positionSeconds: 1 }).catch((reason: unknown) => reason);

    expect(error).toBeInstanceOf(ProgressReportError);
    expect(error).toMatchObject({ status: 503 });
  });

  it("rejects when the server cannot be reached", async () => {
    const send = createProgressSender(
      createFakeApiClient(() => Promise.reject(new TypeError("Failed to fetch"))),
      playerItemId,
    );

    await expect(send({ positionSeconds: 1 })).rejects.toThrow("Failed to fetch");
  });
});
