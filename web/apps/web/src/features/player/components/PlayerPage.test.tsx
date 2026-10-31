import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import { renderApp } from "../../../test/renderApp";
import {
  createdSession,
  createPlaybackServer,
  problemWithRetryAfter,
  sessionResponse,
} from "../test/fakePlaybackServer";
import { playerFileId, playerItemId } from "../test/playerFixtures";

// The page with the real engine and API client against the fake server; the control tests
// next to this file replace the engine hook instead.

const playerPath = `/items/${playerItemId}/play`;

beforeEach(() => {
  // jsdom implements neither; the engine calls both.
  vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue(undefined);
  vi.spyOn(HTMLMediaElement.prototype, "load").mockReturnValue(undefined);
});

afterEach(() => {
  // Unmount first: closing the engine empties the element, which calls load().
  cleanup();
  vi.restoreAllMocks();
});

describe("PlayerPage", () => {
  it("plays the item in a labelled video element and seeks in it", async () => {
    const server = createPlaybackServer();
    renderApp(`${playerPath}?t=30`, { fetch: server.fetch });

    const video = await screen.findByLabelText<HTMLVideoElement>("Video");
    expect(await screen.findByRole("heading", { level: 1, name: "Big Buck Bunny" })).toBeTruthy();
    expect(screen.getByRole("status").textContent).toBe("Loading the video…");
    await vi.waitFor(() => {
      expect(video.getAttribute("src")).toBe("/api/v1/playback/sessions/session-1/file");
    });
    fireEvent(video, new Event("loadedmetadata"));
    fireEvent(video, new Event("loadeddata"));

    await vi.waitFor(() => {
      expect(screen.getByRole("status").textContent).toBe("");
    });
    fireEvent.change(screen.getByRole("slider", { name: "Position" }), {
      target: { value: "50" },
    });
    expect(video.currentTime).toBe(50);
    expect(server.sessionRequests).toHaveLength(1);
  });

  it("plays the file and start position the route's search names", async () => {
    const server = createPlaybackServer();
    renderApp(`${playerPath}?fileId=${playerFileId}&t=90.5`, { fetch: server.fetch });

    await vi.waitFor(() => {
      expect(server.sessionRequests).toHaveLength(1);
    });
    expect(server.sessionRequests[0]).toMatchObject({
      fileId: playerFileId,
      startPositionSeconds: 90.5,
    });
  });

  it("explains a full server in German and retries on request", async () => {
    const server = createPlaybackServer({
      createSession: (request, index) =>
        index === 0
          ? problemWithRetryAfter(503, "session_limit_reached", "5")
          : sessionResponse(createdSession(request, index)),
    });
    renderApp(playerPath, { fetch: server.fetch, languages: ["de"] });

    expect((await screen.findByRole("alert")).textContent).toBe(
      "Auf dem Server laufen gerade zu viele Wiedergaben. Versuche es gleich noch einmal.",
    );
    fireEvent.click(screen.getByRole("button", { name: "Erneut versuchen" }));

    expect(await screen.findByText("Das Video wird geladen …")).toBeTruthy();
    await vi.waitFor(() => {
      expect(server.sessionRequests).toHaveLength(2);
    });
  });

  it("has no serious accessibility violations in its error state", async () => {
    const server = createPlaybackServer({
      createSession: () => problemWithRetryAfter(503, "session_limit_reached", "5"),
    });
    renderApp(playerPath, { fetch: server.fetch });
    await screen.findByRole("button", { name: "Try again" });

    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });
});
